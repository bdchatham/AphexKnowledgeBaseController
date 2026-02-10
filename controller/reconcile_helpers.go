package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
)

// reconcileWorkloadAndService handles the common pattern of reconciling a workload
// (Deployment or StatefulSet) and its associated Service. This eliminates duplication
// across provisioners that all follow the same get-or-create-or-update pattern.
func (r *KnowledgeBaseReconciler) reconcileWorkloadAndService(
	ctx context.Context,
	kb *platformv1alpha1.KnowledgeBase,
	workload client.Object,
	svc *corev1.Service,
	componentName string,
) error {
	logger := log.FromContext(ctx)

	if err := controllerutil.SetControllerReference(kb, workload, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on %s workload: %w", componentName, err)
	}

	if err := r.upsertObject(ctx, workload); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Reconciled %s workload", componentName), "name", workload.GetName())

	if err := controllerutil.SetControllerReference(kb, svc, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on %s service: %w", componentName, err)
	}

	return r.upsertService(ctx, svc)
}

// upsertObject creates or updates a Kubernetes object using the get-or-create-or-update pattern.
func (r *KnowledgeBaseReconciler) upsertObject(ctx context.Context, desired client.Object) error {
	existing, ok := desired.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("failed to deep copy object %s", desired.GetName())
	}
	key := client.ObjectKeyFromObject(desired)

	err := r.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return fmt.Errorf("failed to get %s/%s: %w", desired.GetObjectKind().GroupVersionKind().Kind, key.Name, err)
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

// upsertService creates or updates a Service, preserving ClusterIP on updates.
func (r *KnowledgeBaseReconciler) upsertService(ctx context.Context, desired *corev1.Service) error {
	existing := &corev1.Service{}
	key := client.ObjectKeyFromObject(desired)

	err := r.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return fmt.Errorf("failed to get service %s: %w", key.Name, err)
	}

	desired.ResourceVersion = existing.ResourceVersion
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	return r.Update(ctx, desired)
}

// reconcileUnstructured handles the common pattern for unstructured resources
// (ExternalSecret, HTTPRoute) that follow the same get-or-create-or-update pattern.
func (r *KnowledgeBaseReconciler) reconcileUnstructured(
	ctx context.Context,
	kb *platformv1alpha1.KnowledgeBase,
	desired client.Object,
	componentName string,
) error {
	logger := log.FromContext(ctx)

	if err := controllerutil.SetControllerReference(kb, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on %s: %w", componentName, err)
	}

	if err := r.upsertObject(ctx, desired); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Reconciled %s", componentName), "name", desired.GetName())

	return nil
}

// cleanupWorkloadAndService deletes a workload and its associated service.
func (r *KnowledgeBaseReconciler) cleanupWorkloadAndService(
	ctx context.Context,
	workload client.Object,
	svc client.Object,
) error {
	if err := r.deleteIfExists(ctx, workload); err != nil {
		return err
	}
	return r.deleteIfExists(ctx, svc)
}

// deleteIfExists deletes an object if it exists, ignoring NotFound errors.
func (r *KnowledgeBaseReconciler) deleteIfExists(ctx context.Context, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	if err := r.Get(ctx, key, obj); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get %s for cleanup: %w", key.Name, err)
	}
	if err := r.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s: %w", key.Name, err)
	}
	return nil
}

// reconcileConfigMap handles the common pattern of reconciling a ConfigMap with
// owner reference, get-or-create-or-update semantics.
func (r *KnowledgeBaseReconciler) reconcileConfigMap(
	ctx context.Context,
	kb *platformv1alpha1.KnowledgeBase,
	configMap *corev1.ConfigMap,
	componentName string,
) error {
	if err := controllerutil.SetControllerReference(kb, configMap, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on %s configmap: %w", componentName, err)
	}

	if err := r.upsertObject(ctx, configMap); err != nil {
		return err
	}
	logger := log.FromContext(ctx)
	logger.V(1).Info(fmt.Sprintf("Reconciled %s ConfigMap", componentName), "name", configMap.Name)
	return nil
}
