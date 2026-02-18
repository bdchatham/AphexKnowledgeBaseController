package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
	obj.SetNamespace(infraNamespace(kb))
	obj.SetLabels(httpRouteLabels(kb))
	obj.SetAnnotations(map[string]string{
		"konghq.com/strip-path": "true",
		"konghq.com/plugins":    "mcp-rate-limit,mcp-key-auth",
	})

	obj.Object["spec"] = map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{
				"name":        PlatformGatewayName,
				"namespace":   PlatformGatewayNamespace,
				"sectionName": PlatformAPISectionName,
			},
		},
		"hostnames": []interface{}{PlatformAPIDomain},
		"rules": []interface{}{
			map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "PathPrefix",
							"value": fmt.Sprintf("/mcp/%s", kb.Name),
						},
					},
				},
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name": fmt.Sprintf("mcp-server-%s", kb.Name),
						"port": int64(kb.Spec.MCP.Port),
					},
				},
			},
			map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "PathPrefix",
							"value": fmt.Sprintf("/query/%s", kb.Name),
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
	desired := buildHTTPRoute(kb)
	return r.reconcileUnstructured(ctx, desired, "httproute")
}

func (r *KnowledgeBaseReconciler) cleanupHTTPRoute(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(httpRouteGVK)
	obj.SetName(httpRouteName(kb))
	obj.SetNamespace(infraNamespace(kb))
	return r.deleteIfExists(ctx, obj)
}
