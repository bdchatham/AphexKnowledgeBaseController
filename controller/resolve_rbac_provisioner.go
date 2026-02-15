package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

func resolveRoleBindingName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-resolve", kb.Name)
}

func (r *KnowledgeBaseReconciler) reconcileResolveRBAC(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)
	kbNS := infraNamespace(kb)

	existing := &corev1.ServiceAccount{}
	if err := r.Get(ctx, client.ObjectKey{Name: constants.EventTaskResolverServiceAccount, Namespace: kbNS}, existing); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.EventTaskResolverServiceAccount,
					Namespace: kbNS,
					Labels:    triggerLabels(kb),
				},
			}); err != nil {
				return fmt.Errorf("failed to create resolver SA: %w", err)
			}
		} else {
			return err
		}
	}

	bindingName := resolveRoleBindingName(kb)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: kbNS,
			Labels:    triggerLabels(kb),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     constants.EventTaskResolverRole,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      constants.EventTaskResolverServiceAccount,
			Namespace: kbNS,
		}},
	}

	existingRB := &rbacv1.RoleBinding{}
	err := r.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: kbNS}, existingRB)
	if errors.IsNotFound(err) {
		if err := r.Create(ctx, rb); err != nil {
			return fmt.Errorf("failed to create resolve RoleBinding: %w", err)
		}
		logger.Info("Created resolve RBAC", "namespace", kbNS)
		return nil
	}
	if err != nil {
		return err
	}

	rb.ResourceVersion = existingRB.ResourceVersion
	return r.Update(ctx, rb)
}

func (r *KnowledgeBaseReconciler) cleanupResolveRBAC(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resolveRoleBindingName(kb),
			Namespace: infraNamespace(kb),
		},
	}
	if err := r.Delete(ctx, rb); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete resolve RoleBinding: %w", err)
	}
	return nil
}
