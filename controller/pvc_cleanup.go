package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
)

func (r *KnowledgeBaseReconciler) cleanupPVCs(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	pvcList := &corev1.PersistentVolumeClaimList{}
	listOpts := []client.ListOption{
		client.InNamespace(infraNamespace(kb)),
		client.MatchingLabels{"knowledgebase": kb.Name},
	}

	if err := r.List(ctx, pvcList, listOpts...); err != nil {
		return fmt.Errorf("failed to list PVCs for cleanup: %w", err)
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete PVC %s: %w", pvc.Name, err)
		}
		logger.Info("Deleted PVC", "name", pvc.Name, "namespace", pvc.Namespace)
	}

	return nil
}
