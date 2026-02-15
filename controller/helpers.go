package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	repoMappingConfigMapName = "repo-mapping"

	sourceTypeDocs    = "docs"
	sourceTypeCode    = "code"
	defaultSourcePath = ".kiro/docs"
)

func orgNamespace(kb *platformv1alpha1.KnowledgeBase) string {
	return kb.OrgNamespace()
}

func infraNamespace(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("kb-%s", kb.Name)
}

func (r *KnowledgeBaseReconciler) reconcileNamespace(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	ns := &corev1.Namespace{}
	nsName := infraNamespace(kb)

	err := r.Get(ctx, client.ObjectKey{Name: nsName}, ns)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get namespace %s: %w", nsName, err)
	}

	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Labels: map[string]string{
				constants.LabelOrganization: kb.Spec.Organization,
				constants.LabelManagedBy:    constants.ManagedByKnowledgeBaseController,
				"knowledgebase":             kb.Name,
			},
		},
	}
	if err := r.Create(ctx, ns); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", nsName, err)
	}

	logger := log.FromContext(ctx)
	logger.Info("Created infrastructure namespace", "namespace", nsName)
	return nil
}

func (r *KnowledgeBaseReconciler) cleanupNamespace(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	ns := &corev1.Namespace{}
	nsName := infraNamespace(kb)

	if err := r.Get(ctx, client.ObjectKey{Name: nsName}, ns); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get namespace %s for cleanup: %w", nsName, err)
	}

	if err := r.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete namespace %s: %w", nsName, err)
	}
	return nil
}

// mustParseQuantity parses a resource quantity string and panics on error.
// Safe to use for hardcoded values in controller logic.
func mustParseQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		panic(err)
	}
	return q
}

// isValidBranchName validates Git branch names.
// Git branch names must not contain: spaces, ~, ^, :, ?, *, [, \, .., @{, //
// and must not start or end with a slash or dot.
func isValidBranchName(branch string) bool {
	if branch == "" {
		return false
	}

	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") {
		return false
	}
	if strings.HasPrefix(branch, ".") || strings.HasSuffix(branch, ".") {
		return false
	}

	invalidPatterns := []string{" ", "~", "^", ":", "?", "*", "[", "\\", "..", "@{", "//"}
	for _, pattern := range invalidPatterns {
		if strings.Contains(branch, pattern) {
			return false
		}
	}

	validBranchRegex := regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
	return validBranchRegex.MatchString(branch)
}

// defaultSourceType returns the effective source type, defaulting to "docs"
// when the provided sourceType is empty. This ensures backwards compatibility
// with existing KnowledgeBase resources that predate the sourceType field.
func defaultSourceType(sourceType string) string {
	if sourceType == "" {
		return sourceTypeDocs
	}
	return sourceType
}

// hasCodeSources returns true when any source in the list has sourceType "code".
// Used to determine whether code graph configuration should be included.
func hasCodeSources(sources []platformv1alpha1.Source) bool {
	for _, source := range sources {
		if defaultSourceType(source.SourceType) == sourceTypeCode {
			return true
		}
	}
	return false
}

// buildCELFilter constructs a CEL expression that matches push events
// for the given source repositories using body.repository.full_name.
func buildCELFilter(sources []platformv1alpha1.Source) string {
	if len(sources) == 1 {
		return fmt.Sprintf("body.repository.full_name == '%s'", sources[0].FullName())
	}

	quoted := make([]string, len(sources))
	for i, source := range sources {
		quoted[i] = fmt.Sprintf("'%s'", source.FullName())
	}
	return fmt.Sprintf("body.repository.full_name in [%s]", strings.Join(quoted, ", "))
}

func qdrantStatefulSetName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-qdrant", kb.Name)
}

func qdrantServiceName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-qdrant", kb.Name)
}

func postgresStatefulSetName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-postgres", kb.Name)
}

func postgresServiceName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-postgres", kb.Name)
}

func embeddingDeploymentName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-embedding", kb.Name)
}

func embeddingServiceName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-embedding-svc", kb.Name)
}

func queryDeploymentName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-query", kb.Name)
}

func queryServiceName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-query", kb.Name)
}

func graphDeploymentName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-graph", kb.Name)
}

func graphServiceName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-graph", kb.Name)
}

func appConfigMapName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-config", kb.Name)
}

func externalSecretName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-secrets", kb.Name)
}

func initJobName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-init", kb.Name)
}

func httpRouteName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("%s-route", kb.Name)
}

func secretStoreName(kb *platformv1alpha1.KnowledgeBase) string {
	return fmt.Sprintf("org-%s-store", kb.Spec.Organization)
}


