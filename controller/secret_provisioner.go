package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
	obj.SetNamespace(infraNamespace(kb))
	obj.SetLabels(externalSecretLabels(kb))

	data := []interface{}{
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
	}

	if kb.Spec.Agent != nil {
		data = append(data, map[string]interface{}{
			"secretKey": "auth_credentials",
			"remoteRef": map[string]interface{}{
				"key":      OrgSecretsRemoteKey,
				"property": kb.Spec.Agent.CredentialSecretName,
			},
		})
	}

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
		"data": data,
	}

	return obj
}

func (r *KnowledgeBaseReconciler) reconcileExternalSecret(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	desired := buildExternalSecret(kb)
	return r.reconcileUnstructured(ctx, desired, "external-secret")
}

func (r *KnowledgeBaseReconciler) cleanupExternalSecret(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(externalSecretGVK)
	obj.SetName(externalSecretName(kb))
	obj.SetNamespace(infraNamespace(kb))
	return r.deleteIfExists(ctx, obj)
}
