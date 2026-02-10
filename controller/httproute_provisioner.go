package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

var httpRouteGVK = schema.GroupVersionKind{
	Group:   "gateway.networking.k8s.io",
	Version: "v1",
	Kind:    "HTTPRoute",
}

func httpRouteLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "httproute",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "routing",
	}
}

func buildHTTPRoute(kb *platformv1alpha1.KnowledgeBase) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(httpRouteGVK)
	obj.SetName(httpRouteName(kb))
	obj.SetNamespace(kb.Namespace)
	obj.SetLabels(httpRouteLabels(kb))

	obj.Object["spec"] = map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{
				"name":        PlatformGatewayName,
				"namespace":   PlatformGatewayNamespace,
				"sectionName": PlatformGatewaySectionName,
			},
		},
		"hostnames": []interface{}{
			fmt.Sprintf("%s.home.local", kb.Name),
		},
		"rules": []interface{}{
			map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "PathPrefix",
							"value": "/",
						},
					},
				},
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name": queryServiceName(kb),
						"port": int64(QueryPort),
					},
				},
			},
		},
	}

	return obj
}

func (r *KnowledgeBaseReconciler) reconcileHTTPRoute(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	desired := buildHTTPRoute(kb)
	if err := controllerutil.SetControllerReference(kb, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on httproute: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(httpRouteGVK)

	err := r.Get(ctx, client.ObjectKey{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create httproute: %w", err)
			}
			logger.Info("Created HTTPRoute", "name", desired.GetName())
			return nil
		}
		return fmt.Errorf("failed to get httproute: %w", err)
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	if err := r.Update(ctx, desired); err != nil {
		return fmt.Errorf("failed to update httproute: %w", err)
	}

	return nil
}

func (r *KnowledgeBaseReconciler) cleanupHTTPRoute(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(httpRouteGVK)

	err := r.Get(ctx, client.ObjectKey{Name: httpRouteName(kb), Namespace: kb.Namespace}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get httproute for cleanup: %w", err)
	}

	if err := r.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete httproute: %w", err)
	}

	return nil
}
