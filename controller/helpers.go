package controller

import (
	"fmt"
	"regexp"
	"strings"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	repoMappingNamespace     = "archon"
	repoMappingConfigMapName = "archon-repo-mapping"

	sourceTypeDocs    = "docs"
	sourceTypeCode    = "code"
	defaultSourcePath = ".kiro/docs"
)

// orgNamespace returns the organization namespace for a KnowledgeBase.
// Delegates to the KnowledgeBase type's OrgNamespace() method.
func orgNamespace(kb *platformv1alpha1.KnowledgeBase) string {
	return kb.OrgNamespace()
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
// for the given source repository URLs. A single source uses equality;
// multiple sources use the `in` operator with a list literal.
func buildCELFilter(sources []platformv1alpha1.Source) string {
	if len(sources) == 1 {
		return fmt.Sprintf("body.repository.clone_url == '%s'", sources[0].URL)
	}

	quoted := make([]string, len(sources))
	for i, source := range sources {
		quoted[i] = fmt.Sprintf("'%s'", source.URL)
	}
	return fmt.Sprintf("body.repository.clone_url in [%s]", strings.Join(quoted, ", "))
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
