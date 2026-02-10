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

var externalSecretGVK = schema.GroupVersionKind{
	Group:   "external-secrets.io",
	Version: "v1",
	Kind:    "ExternalSecret",
}

func externalSecretLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "external-secret",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "secrets",
	}
}

func buildExternalSecret(kb *platformv1alpha1.KnowledgeBase) *unstructured.Unstructured {
	name := externalSecretName(kb)
	storeName := secretStoreName(kb)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(externalSecretGVK)
	obj.SetName(name)
	obj.SetNamespace(kb.Namespace)
	obj.SetLabels(externalSecretLabels(kb))

	obj.Object["spec"] = map[string]interface{}{
		"refreshInterval": ExternalSecretRefreshInterval,
		"secretStoreRef": map[string]interface{}{
			"name": storeName,
			"kind": "ClusterSecretStore",
		},
		"target": map[string]interface{}{
			"name":           name,
			"creationPolicy": "Owner",
		},
		"data": []interface{}{
			map[string]interface{}{
				"secretKey": "github_token",
				"remoteRef": map[string]interface{}{
					"key":      OrgSecretsRemoteKey,
					"property": "github-token",
				},
			},
			map[string]interface{}{
				"secretKey": "postgres_user",
				"remoteRef": map[string]interface{}{
					"key":      OrgSecretsRemoteKey,
					"property": "postgres-user",
				},
			},
			map[string]interface{}{
				"secretKey": "postgres_password",
				"remoteRef": map[string]interface{}{
					"key":      OrgSecretsRemoteKey,
					"property": "postgres-password",
				},
			},
		},
	}

	return obj
}

func (r *KnowledgeBaseReconciler) reconcileExternalSecret(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	desired := buildExternalSecret(kb)
	if err := controllerutil.SetControllerReference(kb, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on external secret: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(externalSecretGVK)

	err := r.Get(ctx, client.ObjectKey{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create external secret: %w", err)
			}
			logger.Info("Created ExternalSecret", "name", desired.GetName())
			return nil
		}
		return fmt.Errorf("failed to get external secret: %w", err)
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	if err := r.Update(ctx, desired); err != nil {
		return fmt.Errorf("failed to update external secret: %w", err)
	}

	return nil
}

func (r *KnowledgeBaseReconciler) cleanupExternalSecret(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(externalSecretGVK)

	err := r.Get(ctx, client.ObjectKey{Name: externalSecretName(kb), Namespace: kb.Namespace}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get external secret for cleanup: %w", err)
	}

	if err := r.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete external secret: %w", err)
	}

	return nil
}
