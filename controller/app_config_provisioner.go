package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

func appConfigLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "app-config",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "config",
	}
}

func buildAppConfigMap(kb *platformv1alpha1.KnowledgeBase) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appConfigMapName(kb),
			Namespace: kb.Namespace,
			Labels:    appConfigLabels(kb),
		},
		Data: map[string]string{
			"embedding_service_url": fmt.Sprintf("http://%s:%d", embeddingServiceName(kb), EmbeddingPort),
			"embedding_model":       EmbeddingModel,
			"vector_db_url":         fmt.Sprintf("http://%s:%d", qdrantServiceName(kb), QdrantHTTPPort),
			"collection_name":       CollectionName,
			"postgres_host":         postgresServiceName(kb),
			"postgres_port":         fmt.Sprintf("%d", PostgresPort),
			"postgres_db":           PostgresDBName,
			"retrieval_k":           RetrievalK,
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcileAppConfig(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	configMap := buildAppConfigMap(kb)
	if err := controllerutil.SetControllerReference(kb, configMap, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on app configmap: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: configMap.Name, Namespace: configMap.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, configMap); err != nil {
				return fmt.Errorf("failed to create app configmap: %w", err)
			}
			logger.Info("Created app ConfigMap", "name", configMap.Name)
			return nil
		}
		return fmt.Errorf("failed to get app configmap: %w", err)
	}

	configMap.ResourceVersion = existing.ResourceVersion
	if err := r.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update app configmap: %w", err)
	}

	return nil
}

func (r *KnowledgeBaseReconciler) cleanupAppConfig(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: appConfigMapName(kb), Namespace: kb.Namespace}, configMap); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get app configmap for cleanup: %w", err)
	}

	if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete app configmap: %w", err)
	}

	return nil
}
