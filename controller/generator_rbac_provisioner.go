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

func (r *KnowledgeBaseReconciler) reconcileGeneratorRBAC(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)
	kbNS := infraNamespace(kb)
	saName := constants.KnowledgeGeneratorServiceAccount
	roleName := saName

	sa := &corev1.ServiceAccount{}
	if err := r.Get(ctx, client.ObjectKey{Name: saName, Namespace: kbNS}, sa); errors.IsNotFound(err) {
		if err := r.Create(ctx, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: kbNS, Labels: triggerLabels(kb)},
		}); err != nil {
			return fmt.Errorf("failed to create knowledge-generator SA: %w", err)
		}
	} else if err != nil {
		return err
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: kbNS, Labels: triggerLabels(kb)},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"configmaps", "secrets"},
			Verbs:     []string{"get"},
		}},
	}
	existingRole := &rbacv1.Role{}
	if err := r.Get(ctx, client.ObjectKey{Name: roleName, Namespace: kbNS}, existingRole); errors.IsNotFound(err) {
		if err := r.Create(ctx, role); err != nil {
			return fmt.Errorf("failed to create knowledge-generator Role: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		role.ResourceVersion = existingRole.ResourceVersion
		if err := r.Update(ctx, role); err != nil {
			return err
		}
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: kbNS, Labels: triggerLabels(kb)},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: roleName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: kbNS}},
	}
	existingRB := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, client.ObjectKey{Name: roleName, Namespace: kbNS}, existingRB); errors.IsNotFound(err) {
		if err := r.Create(ctx, rb); err != nil {
			return fmt.Errorf("failed to create knowledge-generator RoleBinding: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		rb.ResourceVersion = existingRB.ResourceVersion
		if err := r.Update(ctx, rb); err != nil {
			return err
		}
	}

	logger.Info("Reconciled knowledge-generator RBAC", "namespace", kbNS)
	return nil
}
