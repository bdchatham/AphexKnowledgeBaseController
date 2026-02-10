package controller

import (
	"context"
	"fmt"

	triggersv1beta1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

func (r *KnowledgeBaseReconciler) reconcileTriggerTemplate(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)
	namespace := orgNamespace(kb)
	templateName := fmt.Sprintf("%s-scip-sync-template", kb.Name)

	triggerTemplate := buildTriggerTemplate(templateName, namespace, kb)

	existing := &triggersv1beta1.TriggerTemplate{}
	err := r.Get(ctx, client.ObjectKey{Name: templateName, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if createErr := r.Create(ctx, triggerTemplate); createErr != nil {
				return fmt.Errorf("failed to create TriggerTemplate: %w", createErr)
			}
			logger.Info("Created TriggerTemplate", "name", templateName, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to get TriggerTemplate: %w", err)
	}

	triggerTemplate.ResourceVersion = existing.ResourceVersion
	if err := r.Update(ctx, triggerTemplate); err != nil {
		return fmt.Errorf("failed to update TriggerTemplate: %w", err)
	}
	logger.V(1).Info("Updated TriggerTemplate", "name", templateName, "namespace", namespace)
	return nil
}

func buildTriggerTemplate(name, namespace string, kb *platformv1alpha1.KnowledgeBase) *triggersv1beta1.TriggerTemplate {
	return &triggersv1beta1.TriggerTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "triggers.tekton.dev/v1beta1",
			Kind:       "TriggerTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    triggerLabels(kb),
		},
		Spec: triggersv1beta1.TriggerTemplateSpec{
			Params: []triggersv1beta1.ParamSpec{
				{Name: "repo-url", Description: "Repository clone URL from push event"},
				{Name: "ref", Description: "Git ref from push event"},
				{Name: "repo-full-name", Description: "Repository full name (org/repo)"},
			},
			ResourceTemplates: []triggersv1beta1.TriggerResourceTemplate{
				{
					RawExtension: runtime.RawExtension{
						Raw: buildResolveAndTriggerTaskRun(namespace),
					},
				},
			},
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcileTrigger(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)
	namespace := orgNamespace(kb)
	triggerName := fmt.Sprintf("%s-scip-sync-trigger", kb.Name)
	templateName := fmt.Sprintf("%s-scip-sync-template", kb.Name)

	trigger := buildTrigger(triggerName, templateName, namespace, kb)

	existing := &triggersv1beta1.Trigger{}
	err := r.Get(ctx, client.ObjectKey{Name: triggerName, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if createErr := r.Create(ctx, trigger); createErr != nil {
				return fmt.Errorf("failed to create Trigger: %w", createErr)
			}
			logger.Info("Created Trigger", "name", triggerName, "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to get Trigger: %w", err)
	}

	trigger.ResourceVersion = existing.ResourceVersion
	if err := r.Update(ctx, trigger); err != nil {
		return fmt.Errorf("failed to update Trigger: %w", err)
	}
	logger.V(1).Info("Updated Trigger", "name", triggerName, "namespace", namespace)
	return nil
}

func buildTrigger(name, templateName, namespace string, kb *platformv1alpha1.KnowledgeBase) *triggersv1beta1.Trigger {
	celFilter := buildCELFilter(kb.Spec.Sources)

	return &triggersv1beta1.Trigger{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "triggers.tekton.dev/v1beta1",
			Kind:       "Trigger",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    triggerLabels(kb),
		},
		Spec: triggersv1beta1.TriggerSpec{
			Interceptors: []*triggersv1beta1.TriggerInterceptor{
				{
					Ref: triggersv1beta1.InterceptorRef{
						Name: "cel",
						Kind: triggersv1beta1.ClusterInterceptorKind,
					},
					Params: []triggersv1beta1.InterceptorParams{
						{
							Name:  "filter",
							Value: apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf("%q", celFilter))},
						},
					},
				},
			},
			Bindings: []*triggersv1beta1.TriggerSpecBinding{
				{Ref: constants.GitHubPushBindingName},
			},
			Template: triggersv1beta1.TriggerSpecTemplate{
				Ref: stringPtr(templateName),
			},
		},
	}
}

func (r *KnowledgeBaseReconciler) cleanupTriggers(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	namespace := orgNamespace(kb)

	if err := r.cleanupTrigger(ctx, kb.Name, namespace); err != nil {
		return err
	}
	return r.cleanupTriggerTemplate(ctx, kb.Name, namespace)
}

func (r *KnowledgeBaseReconciler) cleanupTrigger(ctx context.Context, kbName, namespace string) error {
	triggerName := fmt.Sprintf("%s-scip-sync-trigger", kbName)
	trigger := &triggersv1beta1.Trigger{}

	if err := r.Get(ctx, client.ObjectKey{Name: triggerName, Namespace: namespace}, trigger); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get Trigger: %w", err)
	}

	if err := r.Delete(ctx, trigger); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Trigger: %w", err)
	}
	return nil
}

func (r *KnowledgeBaseReconciler) cleanupTriggerTemplate(ctx context.Context, kbName, namespace string) error {
	templateName := fmt.Sprintf("%s-scip-sync-template", kbName)
	triggerTemplate := &triggersv1beta1.TriggerTemplate{}

	if err := r.Get(ctx, client.ObjectKey{Name: templateName, Namespace: namespace}, triggerTemplate); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get TriggerTemplate: %w", err)
	}

	if err := r.Delete(ctx, triggerTemplate); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete TriggerTemplate: %w", err)
	}
	return nil
}

func triggerLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelOrganization: kb.Spec.Organization,
		constants.LabelManagedBy:    constants.ManagedByKnowledgeBaseController,
		"knowledgebase":             kb.Name,
		"app.kubernetes.io/part-of": "archon",
		"app.kubernetes.io/component": "trigger",
	}
}

func stringPtr(s string) *string {
	return &s
}

