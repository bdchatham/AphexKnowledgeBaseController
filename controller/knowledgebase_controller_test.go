package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
	triggersv1beta1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"pgregory.net/rapid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateSpec_EmptyOrganization(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:         "test-kb",
			Organization: "",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo", SourceType: "docs"},
			},
		},
	}

	err := r.validateSpec(kb)
	if err == nil {
		t.Fatal("expected error for empty organization, got nil")
	}
	if err.Error() != "organization field cannot be empty" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestValidateSpec_EmptySources(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:         "test-kb",
			Organization: "test-org",
			Sources:      []platformv1alpha1.Source{},
		},
	}

	err := r.validateSpec(kb)
	if err == nil {
		t.Fatal("expected error for empty sources, got nil")
	}
	if err.Error() != "sources array cannot be empty" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestValidateSpec_EmptyURL(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:         "test-kb",
			Organization: "test-org",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "", RepoName: "", SourceType: "docs"},
			},
		},
	}

	err := r.validateSpec(kb)
	if err == nil {
		t.Fatal("expected error for empty repoOrg, got nil")
	}
	if err.Error() != "source[0]: repoOrg cannot be empty" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestValidateSpec_EmptyRepoName(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:         "test-kb",
			Organization: "test-org",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "", SourceType: "docs"},
			},
		},
	}

	err := r.validateSpec(kb)
	if err == nil {
		t.Fatal("expected error for empty repoName, got nil")
	}
	if err.Error() != "source[0]: repoName cannot be empty" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestValidateSpec_InvalidBranch(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:         "test-kb",
			Organization: "test-org",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo", Branch: "bad branch name"},
			},
		},
	}

	err := r.validateSpec(kb)
	if err == nil {
		t.Fatal("expected error for invalid branch name, got nil")
	}
}

func TestValidateSpec_ValidSources(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:         "test-kb",
			Organization: "test-org",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "docs", Branch: "main"},
				{RepoOrg: "org", RepoName: "repo-b", SourceType: "code"},
				{RepoOrg: "org", RepoName: "repo-c"},
			},
		},
	}

	err := r.validateSpec(kb)
	if err != nil {
		t.Fatalf("expected no error for valid sources, got: %s", err.Error())
	}
}

func TestValidateSpec_OmittedSourceTypeDefaultsToDocs(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:         "test-kb",
			Organization: "test-org",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo"},
			},
		},
	}

	err := r.validateSpec(kb)
	if err != nil {
		t.Fatalf("expected no error when sourceType is omitted, got: %s", err.Error())
	}
}

func TestBuildRepositoryConfigData_UsesNameField(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name:        "My Knowledge Base",
			Description: "A test KB",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "docs", Branch: "main"},
			},
		},
	}

	data := r.buildRepositoryConfigData(kb)

	if data["name"] != "My Knowledge Base" {
		t.Fatalf("expected name='My Knowledge Base', got %q", data["name"])
	}
	if _, exists := data["displayName"]; exists {
		t.Fatal("expected no 'displayName' key, but it exists")
	}
}

func TestBuildRepositoryConfigData_IncludesSourceType(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "code", Branch: "main"},
				{RepoOrg: "org", RepoName: "repo-b", SourceType: "docs"},
				{RepoOrg: "org", RepoName: "repo-c"},
			},
		},
	}

	data := r.buildRepositoryConfigData(kb)

	if data["sourceCount"] != "3" {
		t.Fatalf("expected sourceCount='3', got %q", data["sourceCount"])
	}
	if _, exists := data["repositoryCount"]; exists {
		t.Fatal("expected no 'repositoryCount' key, but it exists")
	}

	if data["repo.0.sourceType"] != "code" {
		t.Fatalf("expected repo.0.sourceType='code', got %q", data["repo.0.sourceType"])
	}
	if data["repo.1.sourceType"] != "docs" {
		t.Fatalf("expected repo.1.sourceType='docs', got %q", data["repo.1.sourceType"])
	}
	if data["repo.2.sourceType"] != "docs" {
		t.Fatalf("expected repo.2.sourceType='docs' (default), got %q", data["repo.2.sourceType"])
	}
}

func TestBuildRepositoryConfigData_DefaultBranch(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo"},
			},
		},
	}

	data := r.buildRepositoryConfigData(kb)

	if data["repo.0.branch"] != "mainline" {
		t.Fatalf("expected default branch 'mainline', got %q", data["repo.0.branch"])
	}
}

func TestBuildRepositoryConfigData_DefaultPaths(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo"},
			},
		},
	}

	data := r.buildRepositoryConfigData(kb)

	if data["repo.0.paths"] != ".kiro/docs" {
		t.Fatalf("expected default paths '.kiro/docs', got %q", data["repo.0.paths"])
	}
}

func TestBuildRepositoryConfigData_CustomPaths(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo", Paths: []string{"src/**", "lib/**"}},
			},
		},
	}

	data := r.buildRepositoryConfigData(kb)

	if data["repo.0.paths"] != "src/**,lib/**" {
		t.Fatalf("expected paths 'src/**,lib/**', got %q", data["repo.0.paths"])
	}
}

func TestDefaultSourceType_EmptyReturnsDoc(t *testing.T) {
	result := defaultSourceType("")
	if result != "docs" {
		t.Fatalf("expected 'docs' for empty input, got %q", result)
	}
}

func TestDefaultSourceType_ExplicitDocsPassthrough(t *testing.T) {
	result := defaultSourceType("docs")
	if result != "docs" {
		t.Fatalf("expected 'docs', got %q", result)
	}
}

func TestDefaultSourceType_ExplicitCodePassthrough(t *testing.T) {
	result := defaultSourceType("code")
	if result != "code" {
		t.Fatalf("expected 'code', got %q", result)
	}
}

func TestHasCodeSources_ReturnsTrueWhenCodeSourcePresent(t *testing.T) {
	sources := []platformv1alpha1.Source{
		{RepoOrg: "org", RepoName: "repo-a", SourceType: "docs"},
		{RepoOrg: "org", RepoName: "repo-b", SourceType: "code"},
	}

	if !hasCodeSources(sources) {
		t.Fatal("expected true when a code source is present")
	}
}

func TestHasCodeSources_ReturnsFalseWhenAllDocsSources(t *testing.T) {
	sources := []platformv1alpha1.Source{
		{RepoOrg: "org", RepoName: "repo-a", SourceType: "docs"},
		{RepoOrg: "org", RepoName: "repo-b", SourceType: "docs"},
	}

	if hasCodeSources(sources) {
		t.Fatal("expected false when all sources are docs")
	}
}

func TestHasCodeSources_ReturnsFalseWhenOmittedSourceType(t *testing.T) {
	sources := []platformv1alpha1.Source{
		{RepoOrg: "org", RepoName: "repo-a"},
		{RepoOrg: "org", RepoName: "repo-b"},
	}

	if hasCodeSources(sources) {
		t.Fatal("expected false when sourceType is omitted (defaults to docs)")
	}
}

func TestHasCodeSources_ReturnsFalseForEmptySources(t *testing.T) {
	if hasCodeSources([]platformv1alpha1.Source{}) {
		t.Fatal("expected false for empty sources list")
	}
}

func TestBuildSourceConfigData_PerSourceEntries(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "code", Paths: []string{"src/**"}},
				{RepoOrg: "org", RepoName: "repo-b", SourceType: "docs", Paths: []string{".kiro/docs"}},
			},
		},
	}

	data := r.buildSourceConfigData(kb)

	if data["source.0.repoOrg"] != "org" {
		t.Fatalf("expected source.0.repoOrg='org', got %q", data["source.0.repoOrg"])
	}
	if data["source.0.sourceType"] != "code" {
		t.Fatalf("expected source.0.sourceType='code', got %q", data["source.0.sourceType"])
	}
	if data["source.0.paths"] != "src/**" {
		t.Fatalf("expected source.0.paths='src/**', got %q", data["source.0.paths"])
	}
	if data["source.1.repoOrg"] != "org" {
		t.Fatalf("expected source.1.repoOrg='org', got %q", data["source.1.repoOrg"])
	}
	if data["source.1.sourceType"] != "docs" {
		t.Fatalf("expected source.1.sourceType='docs', got %q", data["source.1.sourceType"])
	}
	if data["source.1.paths"] != ".kiro/docs" {
		t.Fatalf("expected source.1.paths='.kiro/docs', got %q", data["source.1.paths"])
	}
}

func TestBuildSourceConfigData_IncludesCodeGraphEndpointWhenCodeSourcePresent(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kb",
			Namespace: "archon",
		},
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "code"},
			},
		},
	}

	data := r.buildSourceConfigData(kb)

	expected := "http://test-kb-graph.kb-test-kb:8081"
	if data["codeGraph.endpoint"] != expected {
		t.Fatalf("expected codeGraph.endpoint=%q, got %q", expected, data["codeGraph.endpoint"])
	}
}

func TestBuildSourceConfigData_OmitsCodeGraphEndpointWhenNoCodeSources(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kb",
			Namespace: "archon",
		},
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "docs"},
			},
		},
	}

	data := r.buildSourceConfigData(kb)

	if _, exists := data["codeGraph.endpoint"]; exists {
		t.Fatal("expected no codeGraph.endpoint when no code sources present")
	}
}

func TestBuildSourceConfigData_DefaultsOmittedSourceTypeToDocs(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a"},
			},
		},
	}

	data := r.buildSourceConfigData(kb)

	if data["source.0.sourceType"] != "docs" {
		t.Fatalf("expected source.0.sourceType='docs' for omitted type, got %q", data["source.0.sourceType"])
	}
}

func TestBuildSourceConfigData_OmitsPathsWhenEmpty(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "docs"},
			},
		},
	}

	data := r.buildSourceConfigData(kb)

	if _, exists := data["source.0.paths"]; exists {
		t.Fatal("expected no source.0.paths when paths is empty")
	}
}

func TestBuildSourceConfigData_MultiplePaths(t *testing.T) {
	r := &KnowledgeBaseReconciler{}
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Name: "test-kb",
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo-a", SourceType: "code", Paths: []string{"src/**", "lib/**", "cmd/**"}},
			},
		},
	}

	data := r.buildSourceConfigData(kb)

	if data["source.0.paths"] != "src/**,lib/**,cmd/**" {
		t.Fatalf("expected source.0.paths='src/**,lib/**,cmd/**', got %q", data["source.0.paths"])
	}
}

// --- Property-Based Tests ---

func sourceGenerator() *rapid.Generator[platformv1alpha1.Source] {
	return rapid.Custom(func(t *rapid.T) platformv1alpha1.Source {
		sourceType := rapid.SampledFrom([]string{"docs", "code", ""}).Draw(t, "sourceType")

		pathCount := rapid.IntRange(0, 5).Draw(t, "pathCount")
		paths := make([]string, pathCount)
		for i := range pathCount {
			paths[i] = rapid.StringMatching(`[a-z][a-z0-9/_.*]{1,20}`).Draw(t, fmt.Sprintf("path_%d", i))
		}

		repoName := rapid.StringMatching(`[a-z][a-z0-9-]{2,15}`).Draw(t, "repoName")

		return platformv1alpha1.Source{
			RepoOrg:    "org",
			RepoName:   repoName,
			SourceType: sourceType,
			Paths:      paths,
		}
	})
}

func knowledgeBaseGenerator() *rapid.Generator[*platformv1alpha1.KnowledgeBase] {
	return rapid.Custom(func(t *rapid.T) *platformv1alpha1.KnowledgeBase {
		sourceCount := rapid.IntRange(1, 10).Draw(t, "sourceCount")
		sources := make([]platformv1alpha1.Source, sourceCount)
		for i := range sourceCount {
			sources[i] = sourceGenerator().Draw(t, fmt.Sprintf("source_%d", i))
		}

		namespace := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "namespace")
		organization := rapid.StringMatching(`[a-z][a-z0-9-]{2,15}`).Draw(t, "organization")

		return &platformv1alpha1.KnowledgeBase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rapid.StringMatching(`[a-z][a-z0-9-]{2,15}`).Draw(t, "kbName"),
				Namespace: namespace,
			},
			Spec: platformv1alpha1.KnowledgeBaseSpec{
				Name:         rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "specName"),
				Organization: organization,
				Sources:      sources,
			},
		}
	})
}

// **Validates: Requirements 4.7**
func TestProperty_DeletionCleansUpAllConfigMaps(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		reposConfigMapName := fmt.Sprintf("%s-repos", kb.Name)
		sourceConfigMapName := fmt.Sprintf("%s-source-config", kb.Name)

		reconciler := &KnowledgeBaseReconciler{}

		repoData := reconciler.buildRepositoryConfigData(kb)
		if repoData == nil {
			t.Fatal("buildRepositoryConfigData returned nil for valid KnowledgeBase")
		}

		sourceData := reconciler.buildSourceConfigData(kb)
		if sourceData == nil {
			t.Fatal("buildSourceConfigData returned nil for valid KnowledgeBase")
		}

		if !strings.HasSuffix(reposConfigMapName, "-repos") {
			t.Fatalf("repos ConfigMap name %q does not follow {kb-name}-repos convention", reposConfigMapName)
		}
		if !strings.HasSuffix(sourceConfigMapName, "-source-config") {
			t.Fatalf("source config ConfigMap name %q does not follow {kb-name}-source-config convention", sourceConfigMapName)
		}

		expectedReposName := kb.Name + "-repos"
		if reposConfigMapName != expectedReposName {
			t.Fatalf("repos ConfigMap name mismatch: expected %q, got %q", expectedReposName, reposConfigMapName)
		}

		expectedSourceConfigName := kb.Name + "-source-config"
		if sourceConfigMapName != expectedSourceConfigName {
			t.Fatalf("source config ConfigMap name mismatch: expected %q, got %q", expectedSourceConfigName, sourceConfigMapName)
		}

		if reposConfigMapName == sourceConfigMapName {
			t.Fatalf("repos and source-config ConfigMap names must be distinct, both are %q", reposConfigMapName)
		}
	})
}

// **Validates: Requirements 4.2, 4.3**
func TestProperty_ConfigMapReconciliationProducesCorrectSourceConfig(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")
		reconciler := &KnowledgeBaseReconciler{}

		data := reconciler.buildSourceConfigData(kb)
		sourceCount := len(kb.Spec.Sources)

		for i, source := range kb.Spec.Sources {
			prefix := fmt.Sprintf("source.%d.", i)

			repoOrg, orgExists := data[prefix+"repoOrg"]
			if !orgExists {
				t.Fatalf("missing %srepoOrg for source %d", prefix, i)
			}
			if repoOrg != source.RepoOrg {
				t.Fatalf("source %d repoOrg mismatch: expected %q, got %q", i, source.RepoOrg, repoOrg)
			}

			repoName, nameExists := data[prefix+"repoName"]
			if !nameExists {
				t.Fatalf("missing %srepoName for source %d", prefix, i)
			}
			if repoName != source.RepoName {
				t.Fatalf("source %d repoName mismatch: expected %q, got %q", i, source.RepoName, repoName)
			}

			sourceType, typeExists := data[prefix+"sourceType"]
			if !typeExists {
				t.Fatalf("missing %ssourceType for source %d", prefix, i)
			}
			expectedType := defaultSourceType(source.SourceType)
			if sourceType != expectedType {
				t.Fatalf("source %d sourceType mismatch: expected %q, got %q", i, expectedType, sourceType)
			}

			if len(source.Paths) > 0 {
				paths, pathsExist := data[prefix+"paths"]
				if !pathsExist {
					t.Fatalf("missing %spaths for source %d with non-empty paths", prefix, i)
				}
				expectedPaths := strings.Join(source.Paths, ",")
				if paths != expectedPaths {
					t.Fatalf("source %d paths mismatch: expected %q, got %q", i, expectedPaths, paths)
				}
			}
		}

		outOfBoundsPrefix := fmt.Sprintf("source.%d.", sourceCount)
		if _, exists := data[outOfBoundsPrefix+"url"]; exists {
			t.Fatalf("unexpected entry at index %d beyond source count %d", sourceCount, sourceCount)
		}

		anyCodeSource := false
		for _, source := range kb.Spec.Sources {
			if defaultSourceType(source.SourceType) == "code" {
				anyCodeSource = true
				break
			}
		}

		_, codeGraphExists := data["codeGraph.endpoint"]
		if anyCodeSource && !codeGraphExists {
			t.Fatal("codeGraph.endpoint missing when code sources are present")
		}
		if !anyCodeSource && codeGraphExists {
			t.Fatal("codeGraph.endpoint present when no code sources exist")
		}
	})
}

// --- Health Check Tests ---

type stubHTTPClient struct {
	statusCode int
	err        error
}

func (s *stubHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.statusCode,
		Body:       http.NoBody,
	}, nil
}

func TestCheckVectorStoreHealth_Success(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{statusCode: 200},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
	}

	r.checkVectorStoreHealth(t.Context(), kb)

	if !kb.Status.VectorStoreReady {
		t.Fatal("expected VectorStoreReady=true after 200 response")
	}
}

func TestCheckVectorStoreHealth_Non2xx(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{statusCode: 503},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
	}

	r.checkVectorStoreHealth(t.Context(), kb)

	if kb.Status.VectorStoreReady {
		t.Fatal("expected VectorStoreReady=false after 503 response")
	}
}

func TestCheckVectorStoreHealth_ConnectionError(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{err: fmt.Errorf("connection refused")},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
	}

	r.checkVectorStoreHealth(t.Context(), kb)

	if kb.Status.VectorStoreReady {
		t.Fatal("expected VectorStoreReady=false after connection error")
	}
}

func TestCheckCodeGraphHealth_Success(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{statusCode: 200},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo", SourceType: "code"},
			},
		},
	}

	r.checkCodeGraphHealth(t.Context(), kb)

	if !kb.Status.CodeGraphReady {
		t.Fatal("expected CodeGraphReady=true after 200 response")
	}
}

func TestCheckCodeGraphHealth_Non2xx(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{statusCode: 500},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo", SourceType: "code"},
			},
		},
	}

	r.checkCodeGraphHealth(t.Context(), kb)

	if kb.Status.CodeGraphReady {
		t.Fatal("expected CodeGraphReady=false after 500 response")
	}
}

func TestCheckCodeGraphHealth_ConnectionError(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{err: fmt.Errorf("connection refused")},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo", SourceType: "code"},
			},
		},
	}

	r.checkCodeGraphHealth(t.Context(), kb)

	if kb.Status.CodeGraphReady {
		t.Fatal("expected CodeGraphReady=false after connection error")
	}
}

func TestCheckCodeGraphHealth_NoCodeSources(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{statusCode: 200},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Sources: []platformv1alpha1.Source{
				{RepoOrg: "org", RepoName: "repo", SourceType: "docs"},
			},
		},
	}

	r.checkCodeGraphHealth(t.Context(), kb)

	if kb.Status.CodeGraphReady {
		t.Fatal("expected CodeGraphReady=false when no code sources exist")
	}
}

func TestCheckCodeGraphHealth_EmptySources(t *testing.T) {
	r := &KnowledgeBaseReconciler{
		HTTPClient: &stubHTTPClient{statusCode: 200},
	}
	kb := &platformv1alpha1.KnowledgeBase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "archon"},
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Sources: []platformv1alpha1.Source{},
		},
	}

	r.checkCodeGraphHealth(t.Context(), kb)

	if kb.Status.CodeGraphReady {
		t.Fatal("expected CodeGraphReady=false when sources are empty")
	}
}

func TestHttpClient_ReturnsInjectedClient(t *testing.T) {
	injected := &stubHTTPClient{statusCode: 200}
	r := &KnowledgeBaseReconciler{HTTPClient: injected}

	got := r.httpClient()
	if got != injected {
		t.Fatal("expected httpClient() to return the injected HTTPClient")
	}
}

func TestHttpClient_ReturnsDefaultWhenNil(t *testing.T) {
	r := &KnowledgeBaseReconciler{}

	got := r.httpClient()
	if got == nil {
		t.Fatal("expected httpClient() to return a non-nil default client")
	}

	if _, ok := got.(*http.Client); !ok {
		t.Fatal("expected default client to be *http.Client")
	}
}

// --- Repo Mapping Tests ---

func TestRemoveEntriesForKnowledgeBase_RemovesMatchingEntries(t *testing.T) {
	data := map[string]string{
		"https://github.com/org/repo-a": "my-kb/archon",
		"https://github.com/org/repo-b": "my-kb/archon",
		"https://github.com/org/repo-c": "other-kb/archon",
	}

	removeEntriesForKnowledgeBase(data, "my-kb/archon")

	if len(data) != 1 {
		t.Fatalf("expected 1 entry remaining, got %d", len(data))
	}
	if data["https://github.com/org/repo-c"] != "other-kb/archon" {
		t.Fatal("expected other-kb entry to remain")
	}
}

func TestRemoveEntriesForKnowledgeBase_NoMatchLeavesDataUnchanged(t *testing.T) {
	data := map[string]string{
		"https://github.com/org/repo-a": "kb-one/ns-one",
		"https://github.com/org/repo-b": "kb-two/ns-two",
	}

	removeEntriesForKnowledgeBase(data, "nonexistent-kb/archon")

	if len(data) != 2 {
		t.Fatalf("expected 2 entries unchanged, got %d", len(data))
	}
}

func TestRemoveEntriesForKnowledgeBase_EmptyMapIsNoOp(t *testing.T) {
	data := map[string]string{}
	removeEntriesForKnowledgeBase(data, "my-kb/archon")

	if len(data) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(data))
	}
}

func TestRemoveEntriesForKnowledgeBase_RemovesAllWhenAllMatch(t *testing.T) {
	data := map[string]string{
		"https://github.com/org/repo-a": "my-kb/archon",
		"https://github.com/org/repo-b": "my-kb/archon",
	}

	removeEntriesForKnowledgeBase(data, "my-kb/archon")

	if len(data) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(data))
	}
}

func TestRepoMappingConstants(t *testing.T) {
	if repoMappingConfigMapName != "repo-mapping" {
		t.Fatalf("expected repoMappingConfigMapName='repo-mapping', got %q", repoMappingConfigMapName)
	}
}

func TestOrgNamespace_ReturnsOrgPrefixedNamespace(t *testing.T) {
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Organization: "my-org",
		},
	}

	result := orgNamespace(kb)
	if result != "org-my-org" {
		t.Fatalf("expected 'org-my-org', got %q", result)
	}
}

func TestOrgNamespace_DelegatesToKBMethod(t *testing.T) {
	kb := &platformv1alpha1.KnowledgeBase{
		Spec: platformv1alpha1.KnowledgeBaseSpec{
			Organization: "test-org",
		},
	}

	helperResult := orgNamespace(kb)
	methodResult := kb.OrgNamespace()
	if helperResult != methodResult {
		t.Fatalf("orgNamespace helper (%q) does not match kb.OrgNamespace() (%q)", helperResult, methodResult)
	}
}

// **Validates: Requirements 2.2, 2.5**
func TestProperty_RepoMappingProducesCorrectEntries(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")
		mappingValue := fmt.Sprintf("%s/%s", kb.Name, fmt.Sprintf("kb-%s", kb.Name))

		data := make(map[string]string)
		for _, source := range kb.Spec.Sources {
			data[source.FullName()] = mappingValue
		}

		for _, source := range kb.Spec.Sources {
			val, exists := data[source.FullName()]
			if !exists {
				t.Fatalf("missing mapping for source FullName %q", source.FullName())
			}
			if val != mappingValue {
				t.Fatalf("mapping value mismatch for %q: expected %q, got %q", source.FullName(), mappingValue, val)
			}
		}

		for _, val := range data {
			if val != mappingValue {
				t.Fatalf("all mapping values should be %q, found %q", mappingValue, val)
			}
		}

		expectedFormat := kb.Name + "/kb-" + kb.Name
		if mappingValue != expectedFormat {
			t.Fatalf("mapping value format mismatch: expected %q, got %q", expectedFormat, mappingValue)
		}
	})
}

// **Validates: Requirements 3.1, 3.2, 3.3, 7.3**
func TestProperty_TriggerProvisioningProducesCorrectResources(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		expectedNamespace := fmt.Sprintf("org-%s", kb.Spec.Organization)
		expectedTemplateName := fmt.Sprintf("%s-scip-sync-template", kb.Name)
		expectedTriggerName := fmt.Sprintf("%s-scip-sync-trigger", kb.Name)

		triggerTemplate := buildTriggerTemplate(expectedTemplateName, expectedNamespace, kb)
		trigger := buildTrigger(expectedTriggerName, expectedTemplateName, expectedNamespace, kb)

		// (a) TriggerTemplate name and namespace
		if triggerTemplate.Name != expectedTemplateName {
			t.Fatalf("TriggerTemplate name: expected %q, got %q", expectedTemplateName, triggerTemplate.Name)
		}
		if triggerTemplate.Namespace != expectedNamespace {
			t.Fatalf("TriggerTemplate namespace: expected %q, got %q", expectedNamespace, triggerTemplate.Namespace)
		}

		// (b) Trigger name and namespace
		if trigger.Name != expectedTriggerName {
			t.Fatalf("Trigger name: expected %q, got %q", expectedTriggerName, trigger.Name)
		}
		if trigger.Namespace != expectedNamespace {
			t.Fatalf("Trigger namespace: expected %q, got %q", expectedNamespace, trigger.Namespace)
		}

		// (b) Trigger CEL filter contains all source FullNames
		celFilter := buildCELFilter(kb.Spec.Sources)
		for _, source := range kb.Spec.Sources {
			if !strings.Contains(celFilter, source.FullName()) {
				t.Fatalf("CEL filter %q does not contain source FullName %q", celFilter, source.FullName())
			}
		}

		// (c) Trigger references github-push-binding
		if len(trigger.Spec.Bindings) == 0 {
			t.Fatal("Trigger has no bindings")
		}
		foundBinding := false
		for _, binding := range trigger.Spec.Bindings {
			if binding.Ref == "github-push-binding" {
				foundBinding = true
				break
			}
		}
		if !foundBinding {
			t.Fatal("Trigger does not reference github-push-binding in bindings")
		}

		// (d) Both resources carry knowledgebase label matching KB name
		if triggerTemplate.Labels["knowledgebase"] != kb.Name {
			t.Fatalf("TriggerTemplate knowledgebase label: expected %q, got %q", kb.Name, triggerTemplate.Labels["knowledgebase"])
		}
		if trigger.Labels["knowledgebase"] != kb.Name {
			t.Fatalf("Trigger knowledgebase label: expected %q, got %q", kb.Name, trigger.Labels["knowledgebase"])
		}

		// Both resources carry platform.aphex/organization label matching org
		if triggerTemplate.Labels["platform.aphex/organization"] != kb.Spec.Organization {
			t.Fatalf("TriggerTemplate organization label: expected %q, got %q", kb.Spec.Organization, triggerTemplate.Labels["platform.aphex/organization"])
		}
		if trigger.Labels["platform.aphex/organization"] != kb.Spec.Organization {
			t.Fatalf("Trigger organization label: expected %q, got %q", kb.Spec.Organization, trigger.Labels["platform.aphex/organization"])
		}

		// Both resources carry platform.aphex/managed-by label = knowledgebase-controller
		if triggerTemplate.Labels["platform.aphex/managed-by"] != "knowledgebase-controller" {
			t.Fatalf("TriggerTemplate managed-by label: expected %q, got %q", "knowledgebase-controller", triggerTemplate.Labels["platform.aphex/managed-by"])
		}
		if trigger.Labels["platform.aphex/managed-by"] != "knowledgebase-controller" {
			t.Fatalf("Trigger managed-by label: expected %q, got %q", "knowledgebase-controller", trigger.Labels["platform.aphex/managed-by"])
		}
	})
}

// **Validates: Requirements 2.2, 2.5**
func TestProperty_RepoMappingCleanupRemovesOnlyTargetKB(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kbCount := rapid.IntRange(2, 5).Draw(t, "kbCount")

		type kbEntry struct {
			name      string
			namespace string
			urls      []string
		}

		entries := make([]kbEntry, kbCount)
		data := make(map[string]string)

		for i := range kbCount {
			name := rapid.StringMatching(`[a-z][a-z0-9-]{2,10}`).Draw(t, fmt.Sprintf("kbName_%d", i))
			namespace := rapid.StringMatching(`[a-z]{3,8}`).Draw(t, fmt.Sprintf("kbNs_%d", i))
			urlCount := rapid.IntRange(1, 4).Draw(t, fmt.Sprintf("urlCount_%d", i))

			urls := make([]string, urlCount)
			for j := range urlCount {
				repoName := rapid.StringMatching(`[a-z][a-z0-9-]{2,10}`).Draw(t, fmt.Sprintf("repo_%d_%d", i, j))
				urls[j] = fmt.Sprintf("https://github.com/%s/%s", name, repoName)
			}

			entries[i] = kbEntry{name: name, namespace: namespace, urls: urls}
			mappingValue := fmt.Sprintf("%s/%s", name, namespace)
			for _, url := range urls {
				data[url] = mappingValue
			}
		}

		targetIdx := rapid.IntRange(0, kbCount-1).Draw(t, "targetIdx")
		targetValue := fmt.Sprintf("%s/%s", entries[targetIdx].name, entries[targetIdx].namespace)

		totalBefore := len(data)
		targetCountBefore := 0
		for _, val := range data {
			if val == targetValue {
				targetCountBefore++
			}
		}

		removeEntriesForKnowledgeBase(data, targetValue)

		for _, val := range data {
			if val == targetValue {
				t.Fatalf("found entry with value %q after cleanup", targetValue)
			}
		}

		expectedRemaining := totalBefore - targetCountBefore
		if len(data) != expectedRemaining {
			t.Fatalf("expected %d entries remaining, got %d", expectedRemaining, len(data))
		}
	})
}

// Feature: eventlistener-migration, Property 7: CEL filter reflects current sources
// **Validates: Requirements 3.6**
func TestProperty_CELFilterReflectsCurrentSources(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		oldCount := rapid.IntRange(1, 10).Draw(t, "oldSourceCount")
		newCount := rapid.IntRange(1, 10).Draw(t, "newSourceCount")

		usedRepoNames := make(map[string]bool)

		generateDistinctSources := func(count int, label string) []platformv1alpha1.Source {
			sources := make([]platformv1alpha1.Source, count)
			for i := range count {
				var repoName string
				for {
					repoName = rapid.StringMatching(`[a-z][a-z0-9-]{2,15}`).Draw(t, fmt.Sprintf("%s_repo_%d", label, i))
					if !usedRepoNames[repoName] {
						usedRepoNames[repoName] = true
						break
					}
				}
				sources[i] = platformv1alpha1.Source{
					RepoOrg:  "org",
					RepoName: repoName,
				}
			}
			return sources
		}

		oldSources := generateDistinctSources(oldCount, "old")
		newSources := generateDistinctSources(newCount, "new")

		oldFilter := buildCELFilter(oldSources)
		for _, source := range oldSources {
			if !strings.Contains(oldFilter, source.FullName()) {
				t.Fatalf("old CEL filter %q does not contain source FullName %q", oldFilter, source.FullName())
			}
		}

		newFilter := buildCELFilter(newSources)
		for _, source := range newSources {
			if !strings.Contains(newFilter, source.FullName()) {
				t.Fatalf("new CEL filter %q does not contain source FullName %q", newFilter, source.FullName())
			}
		}

		removedNames := make(map[string]bool)
		for _, source := range oldSources {
			removedNames[source.FullName()] = true
		}
		for _, source := range newSources {
			delete(removedNames, source.FullName())
		}

		for removedURL := range removedNames {
			quotedURL := fmt.Sprintf("'%s'", removedURL)
			if strings.Contains(newFilter, quotedURL) {
				t.Fatalf("new CEL filter %q still contains removed URL %q", newFilter, removedURL)
			}
		}
	})
}

// Feature: eventlistener-migration, Property 8: Multi-org namespace isolation
// **Validates: Requirements 7.1**
func TestProperty_MultiOrgNamespaceIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb1 := knowledgeBaseGenerator().Draw(t, "kb1")
		kb2 := knowledgeBaseGenerator().Draw(t, "kb2")

		for kb2.Spec.Organization == kb1.Spec.Organization {
			kb2 = knowledgeBaseGenerator().Draw(t, "kb2_retry")
		}

		ns1 := orgNamespace(kb1)
		ns2 := orgNamespace(kb2)

		if ns1 == ns2 {
			t.Fatalf("expected different namespaces for different orgs, both got %q", ns1)
		}

		expectedNs1 := fmt.Sprintf("org-%s", kb1.Spec.Organization)
		expectedNs2 := fmt.Sprintf("org-%s", kb2.Spec.Organization)

		if ns1 != expectedNs1 {
			t.Fatalf("KB1 namespace: expected %q, got %q", expectedNs1, ns1)
		}
		if ns2 != expectedNs2 {
			t.Fatalf("KB2 namespace: expected %q, got %q", expectedNs2, ns2)
		}

		templateName1 := fmt.Sprintf("%s-scip-sync-template", kb1.Name)
		triggerName1 := fmt.Sprintf("%s-scip-sync-trigger", kb1.Name)
		tt1 := buildTriggerTemplate(templateName1, ns1, kb1)
		tr1 := buildTrigger(triggerName1, templateName1, ns1, kb1)

		templateName2 := fmt.Sprintf("%s-scip-sync-template", kb2.Name)
		triggerName2 := fmt.Sprintf("%s-scip-sync-trigger", kb2.Name)
		tt2 := buildTriggerTemplate(templateName2, ns2, kb2)
		tr2 := buildTrigger(triggerName2, templateName2, ns2, kb2)

		if tt1.Namespace != expectedNs1 {
			t.Fatalf("KB1 TriggerTemplate namespace: expected %q, got %q", expectedNs1, tt1.Namespace)
		}
		if tr1.Namespace != expectedNs1 {
			t.Fatalf("KB1 Trigger namespace: expected %q, got %q", expectedNs1, tr1.Namespace)
		}
		if tt2.Namespace != expectedNs2 {
			t.Fatalf("KB2 TriggerTemplate namespace: expected %q, got %q", expectedNs2, tt2.Namespace)
		}
		if tr2.Namespace != expectedNs2 {
			t.Fatalf("KB2 Trigger namespace: expected %q, got %q", expectedNs2, tr2.Namespace)
		}

		if tt1.Labels[constants.LabelOrganization] != kb1.Spec.Organization {
			t.Fatalf("KB1 TriggerTemplate org label: expected %q, got %q",
				kb1.Spec.Organization, tt1.Labels[constants.LabelOrganization])
		}
		if tr1.Labels[constants.LabelOrganization] != kb1.Spec.Organization {
			t.Fatalf("KB1 Trigger org label: expected %q, got %q",
				kb1.Spec.Organization, tr1.Labels[constants.LabelOrganization])
		}
		if tt2.Labels[constants.LabelOrganization] != kb2.Spec.Organization {
			t.Fatalf("KB2 TriggerTemplate org label: expected %q, got %q",
				kb2.Spec.Organization, tt2.Labels[constants.LabelOrganization])
		}
		if tr2.Labels[constants.LabelOrganization] != kb2.Spec.Organization {
			t.Fatalf("KB2 Trigger org label: expected %q, got %q",
				kb2.Spec.Organization, tr2.Labels[constants.LabelOrganization])
		}

		if tr1.Labels[constants.LabelOrganization] == tr2.Labels[constants.LabelOrganization] {
			t.Fatalf("cross-contamination: KB1 and KB2 Trigger org labels are both %q",
				tr1.Labels[constants.LabelOrganization])
		}
		if tt1.Labels[constants.LabelOrganization] == tt2.Labels[constants.LabelOrganization] {
			t.Fatalf("cross-contamination: KB1 and KB2 TriggerTemplate org labels are both %q",
				tt1.Labels[constants.LabelOrganization])
		}
	})
}

// Feature: kb-infra-provisioning, Property 9: Resource Naming Uniqueness
// **Validates: Requirements 12.1, 12.2**
func TestProperty_ResourceNamingUniqueness(t *testing.T) {
	type namingFunc struct {
		label string
		fn    func(*platformv1alpha1.KnowledgeBase) string
	}

	namingFuncs := []namingFunc{
		{"qdrantStatefulSet", qdrantStatefulSetName},
		{"qdrantService", qdrantServiceName},
		{"postgresStatefulSet", postgresStatefulSetName},
		{"postgresService", postgresServiceName},
		{"embeddingDeployment", embeddingDeploymentName},
		{"embeddingService", embeddingServiceName},
		{"queryDeployment", queryDeploymentName},
		{"queryService", queryServiceName},
		{"appConfigMap", appConfigMapName},
		{"externalSecret", externalSecretName},
		{"initJob", initJobName},
		{"httpRoute", httpRouteName},
	}

	t.Run("all names start with KB name prefix", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

			for _, nf := range namingFuncs {
				name := nf.fn(kb)
				if !strings.HasPrefix(name, kb.Name) {
					t.Fatalf("%s name %q does not start with KB name prefix %q", nf.label, name, kb.Name)
				}
			}
		})
	})

	t.Run("distinct KB names produce distinct resource names", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kb1 := knowledgeBaseGenerator().Draw(t, "kb1")
			kb2 := knowledgeBaseGenerator().Draw(t, "kb2")

			for kb2.Name == kb1.Name {
				kb2 = knowledgeBaseGenerator().Draw(t, "kb2_retry")
			}

			for _, nf := range namingFuncs {
				name1 := nf.fn(kb1)
				name2 := nf.fn(kb2)
				if name1 == name2 {
					t.Fatalf("%s produced identical names %q for distinct KBs %q and %q",
						nf.label, name1, kb1.Name, kb2.Name)
				}
			}
		})
	})
}

// Feature: eventlistener-migration, Property 6: Trigger cleanup removes only owned resources
// **Validates: Requirements 3.5, 7.2**
func TestProperty_TriggerCleanupRemovesOnlyOwnedResources(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sharedOrg := rapid.StringMatching(`[a-z][a-z0-9-]{2,15}`).Draw(t, "sharedOrg")
		orgNs := fmt.Sprintf("org-%s", sharedOrg)

		kbCount := rapid.IntRange(2, 5).Draw(t, "kbCount")

		knowledgeBases := make([]*platformv1alpha1.KnowledgeBase, kbCount)
		usedNames := make(map[string]bool)
		for i := range kbCount {
			var kb *platformv1alpha1.KnowledgeBase
			for {
				kb = knowledgeBaseGenerator().Draw(t, fmt.Sprintf("kb_%d", i))
				kb.Spec.Organization = sharedOrg
				if !usedNames[kb.Name] {
					usedNames[kb.Name] = true
					break
				}
			}
			knowledgeBases[i] = kb
		}

		initialObjects := make([]client.Object, 0, 2*len(knowledgeBases))
		for _, kb := range knowledgeBases {
			templateName := fmt.Sprintf("%s-scip-sync-template", kb.Name)
			triggerName := fmt.Sprintf("%s-scip-sync-trigger", kb.Name)

			tt := buildTriggerTemplate(templateName, orgNs, kb)
			tr := buildTrigger(triggerName, templateName, orgNs, kb)

			initialObjects = append(initialObjects, tt, tr)
		}

		scheme := runtime.NewScheme()
		if err := triggersv1beta1.AddToScheme(scheme); err != nil {
			t.Fatalf("failed to add triggers scheme: %v", err)
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(initialObjects...).
			Build()

		reconciler := &KnowledgeBaseReconciler{
			Client: fakeClient,
		}

		targetIdx := rapid.IntRange(0, kbCount-1).Draw(t, "targetIdx")
		targetKB := knowledgeBases[targetIdx]

		ctx := context.Background()
		err := reconciler.cleanupTriggers(ctx, targetKB)
		if err != nil {
			t.Fatalf("cleanupTriggers failed: %v", err)
		}

		deletedTrigger := &triggersv1beta1.Trigger{}
		deletedTriggerKey := client.ObjectKey{
			Name:      fmt.Sprintf("%s-scip-sync-trigger", targetKB.Name),
			Namespace: orgNs,
		}
		if err := fakeClient.Get(ctx, deletedTriggerKey, deletedTrigger); err == nil {
			t.Fatalf("expected Trigger %q to be deleted, but it still exists", deletedTriggerKey.Name)
		}

		deletedTemplate := &triggersv1beta1.TriggerTemplate{}
		deletedTemplateKey := client.ObjectKey{
			Name:      fmt.Sprintf("%s-scip-sync-template", targetKB.Name),
			Namespace: orgNs,
		}
		if err := fakeClient.Get(ctx, deletedTemplateKey, deletedTemplate); err == nil {
			t.Fatalf("expected TriggerTemplate %q to be deleted, but it still exists", deletedTemplateKey.Name)
		}

		for i, kb := range knowledgeBases {
			if i == targetIdx {
				continue
			}

			survivingTrigger := &triggersv1beta1.Trigger{}
			triggerKey := client.ObjectKey{
				Name:      fmt.Sprintf("%s-scip-sync-trigger", kb.Name),
				Namespace: orgNs,
			}
			if err := fakeClient.Get(ctx, triggerKey, survivingTrigger); err != nil {
				t.Fatalf("expected Trigger %q from KB %q to survive cleanup, but got error: %v",
					triggerKey.Name, kb.Name, err)
			}

			survivingTemplate := &triggersv1beta1.TriggerTemplate{}
			templateKey := client.ObjectKey{
				Name:      fmt.Sprintf("%s-scip-sync-template", kb.Name),
				Namespace: orgNs,
			}
			if err := fakeClient.Get(ctx, templateKey, survivingTemplate); err != nil {
				t.Fatalf("expected TriggerTemplate %q from KB %q to survive cleanup, but got error: %v",
					templateKey.Name, kb.Name, err)
			}
		}
	})
}

// Feature: kb-infra-provisioning, Property 1: Qdrant Builder Correctness
// **Validates: Requirements 1.1, 1.2, 1.4**
func TestProperty_QdrantBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		ss := buildQdrantStatefulSet(kb)
		svc := buildQdrantService(kb)

		expectedName := fmt.Sprintf("%s-qdrant", kb.Name)

		// StatefulSet name and namespace
		if ss.Name != expectedName {
			t.Fatalf("StatefulSet name: expected %q, got %q", expectedName, ss.Name)
		}
		if ss.Namespace != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("StatefulSet namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), ss.Namespace)
		}

		// Container image
		container := ss.Spec.Template.Spec.Containers[0]
		if container.Image != QdrantImage {
			t.Fatalf("container image: expected %q, got %q", QdrantImage, container.Image)
		}

		// Container ports (6333 and 6334)
		portMap := make(map[int32]bool)
		for _, p := range container.Ports {
			portMap[p.ContainerPort] = true
		}
		if !portMap[int32(QdrantHTTPPort)] {
			t.Fatalf("container missing HTTP port %d", QdrantHTTPPort)
		}
		if !portMap[int32(QdrantGRPCPort)] {
			t.Fatalf("container missing gRPC port %d", QdrantGRPCPort)
		}

		// VolumeClaimTemplate requests 10Gi storage
		if len(ss.Spec.VolumeClaimTemplates) != 1 {
			t.Fatalf("expected 1 VolumeClaimTemplate, got %d", len(ss.Spec.VolumeClaimTemplates))
		}
		storageReq := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		expectedStorage := resource.MustParse(QdrantStorageSize)
		if storageReq.Cmp(expectedStorage) != 0 {
			t.Fatalf("VCT storage: expected %s, got %s", expectedStorage.String(), storageReq.String())
		}

		// Memory request 512Mi, limit 1Gi
		memReq := container.Resources.Requests[corev1.ResourceMemory]
		expectedMemReq := mustParseQuantity(QdrantMemoryRequest)
		if memReq.Cmp(expectedMemReq) != 0 {
			t.Fatalf("memory request: expected %s, got %s", expectedMemReq.String(), memReq.String())
		}
		memLimit := container.Resources.Limits[corev1.ResourceMemory]
		expectedMemLimit := mustParseQuantity(QdrantMemoryLimit)
		if memLimit.Cmp(expectedMemLimit) != 0 {
			t.Fatalf("memory limit: expected %s, got %s", expectedMemLimit.String(), memLimit.String())
		}

		// CPU request 250m
		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		expectedCPUReq := mustParseQuantity(QdrantCPURequest)
		if cpuReq.Cmp(expectedCPUReq) != 0 {
			t.Fatalf("CPU request: expected %s, got %s", expectedCPUReq.String(), cpuReq.String())
		}

		// Liveness probe targeting port 6333
		if container.LivenessProbe == nil {
			t.Fatal("liveness probe is nil")
		}
		if container.LivenessProbe.HTTPGet == nil {
			t.Fatal("liveness probe HTTPGet is nil")
		}
		livenessPort := container.LivenessProbe.HTTPGet.Port.IntValue()
		if livenessPort != QdrantHTTPPort {
			t.Fatalf("liveness probe port: expected %d, got %d", QdrantHTTPPort, livenessPort)
		}

		// Readiness probe targeting port 6333
		if container.ReadinessProbe == nil {
			t.Fatal("readiness probe is nil")
		}
		if container.ReadinessProbe.HTTPGet == nil {
			t.Fatal("readiness probe HTTPGet is nil")
		}
		readinessPort := container.ReadinessProbe.HTTPGet.Port.IntValue()
		if readinessPort != QdrantHTTPPort {
			t.Fatalf("readiness probe port: expected %d, got %d", QdrantHTTPPort, readinessPort)
		}

		// Volume mount at /qdrant/storage
		foundMount := false
		for _, vm := range container.VolumeMounts {
			if vm.MountPath == "/qdrant/storage" {
				foundMount = true
				break
			}
		}
		if !foundMount {
			t.Fatal("missing volume mount at /qdrant/storage")
		}

		// Service name and type
		if svc.Name != expectedName {
			t.Fatalf("Service name: expected %q, got %q", expectedName, svc.Name)
		}
		if svc.Spec.Type != corev1.ServiceTypeClusterIP {
			t.Fatalf("Service type: expected ClusterIP, got %s", svc.Spec.Type)
		}

		// Service exposes ports 6333 and 6334
		svcPortMap := make(map[int32]bool)
		for _, p := range svc.Spec.Ports {
			svcPortMap[p.Port] = true
		}
		if !svcPortMap[int32(QdrantHTTPPort)] {
			t.Fatalf("Service missing HTTP port %d", QdrantHTTPPort)
		}
		if !svcPortMap[int32(QdrantGRPCPort)] {
			t.Fatalf("Service missing gRPC port %d", QdrantGRPCPort)
		}
	})
}

// Feature: kb-infra-provisioning, Property 3: Init Job Builder Correctness
// **Validates: Requirements 3.1, 3.2, 3.3, 3.5, 14.1, 14.2**
func TestProperty_InitJobBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		job := buildInitJob(kb)

		expectedJobName := fmt.Sprintf("%s-init", kb.Name)
		expectedPgHost := fmt.Sprintf("%s-postgres", kb.Name)
		expectedQdrantHost := fmt.Sprintf("%s-qdrant", kb.Name)

		// 1. Job name is {kb-name}-init
		if job.Name != expectedJobName {
			t.Fatalf("Job name: expected %q, got %q", expectedJobName, job.Name)
		}

		// 2. Job namespace matches KB namespace
		if job.Namespace != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("Job namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), job.Namespace)
		}

		// 3. Has exactly 2 containers
		containers := job.Spec.Template.Spec.Containers
		if len(containers) != 2 {
			t.Fatalf("expected 2 containers, got %d", len(containers))
		}

		containerMap := make(map[string]corev1.Container)
		for _, c := range containers {
			containerMap[c.Name] = c
		}

		// 4. Container named init-postgres exists with image postgres:15-alpine
		initPostgres, exists := containerMap["init-postgres"]
		if !exists {
			t.Fatal("missing container named init-postgres")
		}
		if initPostgres.Image != InitPostgresImage {
			t.Fatalf("init-postgres image: expected %q, got %q", InitPostgresImage, initPostgres.Image)
		}

		// 5. Container named init-qdrant exists with image curlimages/curl:8.5.0
		initQdrant, exists := containerMap["init-qdrant"]
		if !exists {
			t.Fatal("missing container named init-qdrant")
		}
		if initQdrant.Image != InitCurlImage {
			t.Fatalf("init-qdrant image: expected %q, got %q", InitCurlImage, initQdrant.Image)
		}

		// Extract command strings for content checks
		pgCommand := strings.Join(initPostgres.Command, " ")
		qdrantCommand := strings.Join(initQdrant.Command, " ")

		// 6. init-postgres command contains CREATE TABLE IF NOT EXISTS document_state
		if !strings.Contains(pgCommand, "CREATE TABLE IF NOT EXISTS document_state") {
			t.Fatal("init-postgres command missing 'CREATE TABLE IF NOT EXISTS document_state'")
		}

		// 7. init-postgres command contains CREATE INDEX IF NOT EXISTS
		if !strings.Contains(pgCommand, "CREATE INDEX IF NOT EXISTS") {
			t.Fatal("init-postgres command missing 'CREATE INDEX IF NOT EXISTS'")
		}

		// 8. init-qdrant command contains a PUT request
		if !strings.Contains(qdrantCommand, "PUT") {
			t.Fatal("init-qdrant command missing PUT request")
		}

		// 9. init-qdrant command contains 768 (vector size)
		if !strings.Contains(qdrantCommand, fmt.Sprintf("%d", QdrantVectorSize)) {
			t.Fatalf("init-qdrant command missing vector size %d", QdrantVectorSize)
		}

		// 10. init-qdrant command contains Cosine (distance metric)
		if !strings.Contains(qdrantCommand, QdrantDistance) {
			t.Fatalf("init-qdrant command missing distance metric %q", QdrantDistance)
		}

		// 11. init-postgres env PGHOST references {kb-name}-postgres
		pgEnvMap := make(map[string]corev1.EnvVar)
		for _, env := range initPostgres.Env {
			pgEnvMap[env.Name] = env
		}
		pgHostEnv, exists := pgEnvMap["PGHOST"]
		if !exists {
			t.Fatal("init-postgres missing PGHOST environment variable")
		}
		if pgHostEnv.Value != expectedPgHost {
			t.Fatalf("PGHOST: expected %q, got %q", expectedPgHost, pgHostEnv.Value)
		}

		// 12. init-qdrant command references {kb-name}-qdrant
		if !strings.Contains(qdrantCommand, expectedQdrantHost) {
			t.Fatalf("init-qdrant command missing qdrant host reference %q", expectedQdrantHost)
		}

		// 13. RestartPolicy is OnFailure
		if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyOnFailure {
			t.Fatalf("RestartPolicy: expected %q, got %q",
				corev1.RestartPolicyOnFailure, job.Spec.Template.Spec.RestartPolicy)
		}
	})
}

// Feature: kb-infra-provisioning, Property 2: Postgres Builder Correctness
// **Validates: Requirements 2.1, 2.2, 2.4**
func TestProperty_PostgresBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		ss := buildPostgresStatefulSet(kb)
		svc := buildPostgresService(kb)

		expectedName := fmt.Sprintf("%s-postgres", kb.Name)
		expectedSecretName := fmt.Sprintf("%s-secrets", kb.Name)

		// StatefulSet name and namespace
		if ss.Name != expectedName {
			t.Fatalf("StatefulSet name: expected %q, got %q", expectedName, ss.Name)
		}
		if ss.Namespace != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("StatefulSet namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), ss.Namespace)
		}

		// Container image
		container := ss.Spec.Template.Spec.Containers[0]
		if container.Image != PostgresImage {
			t.Fatalf("container image: expected %q, got %q", PostgresImage, container.Image)
		}

		// Container port 5432
		portMap := make(map[int32]bool)
		for _, p := range container.Ports {
			portMap[p.ContainerPort] = true
		}
		if !portMap[int32(PostgresPort)] {
			t.Fatalf("container missing port %d", PostgresPort)
		}

		// VolumeClaimTemplate requests 5Gi storage
		if len(ss.Spec.VolumeClaimTemplates) != 1 {
			t.Fatalf("expected 1 VolumeClaimTemplate, got %d", len(ss.Spec.VolumeClaimTemplates))
		}
		storageReq := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		expectedStorage := resource.MustParse(PostgresStorageSize)
		if storageReq.Cmp(expectedStorage) != 0 {
			t.Fatalf("VCT storage: expected %s, got %s", expectedStorage.String(), storageReq.String())
		}

		// Memory request 256Mi, limit 512Mi
		memReq := container.Resources.Requests[corev1.ResourceMemory]
		expectedMemReq := mustParseQuantity(PostgresMemoryRequest)
		if memReq.Cmp(expectedMemReq) != 0 {
			t.Fatalf("memory request: expected %s, got %s", expectedMemReq.String(), memReq.String())
		}
		memLimit := container.Resources.Limits[corev1.ResourceMemory]
		expectedMemLimit := mustParseQuantity(PostgresMemoryLimit)
		if memLimit.Cmp(expectedMemLimit) != 0 {
			t.Fatalf("memory limit: expected %s, got %s", expectedMemLimit.String(), memLimit.String())
		}

		// CPU request 100m
		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		expectedCPUReq := mustParseQuantity(PostgresCPURequest)
		if cpuReq.Cmp(expectedCPUReq) != 0 {
			t.Fatalf("CPU request: expected %s, got %s", expectedCPUReq.String(), cpuReq.String())
		}

		// POSTGRES_DB env set to "archon"
		envMap := make(map[string]corev1.EnvVar)
		for _, env := range container.Env {
			envMap[env.Name] = env
		}
		dbEnv, exists := envMap["POSTGRES_DB"]
		if !exists {
			t.Fatal("missing POSTGRES_DB environment variable")
		}
		if dbEnv.Value != PostgresDBName {
			t.Fatalf("POSTGRES_DB: expected %q, got %q", PostgresDBName, dbEnv.Value)
		}

		// POSTGRES_USER sourced from {kb-name}-secrets
		userEnv, exists := envMap["POSTGRES_USER"]
		if !exists {
			t.Fatal("missing POSTGRES_USER environment variable")
		}
		if userEnv.ValueFrom == nil || userEnv.ValueFrom.SecretKeyRef == nil {
			t.Fatal("POSTGRES_USER must be sourced from a Secret")
		}
		if userEnv.ValueFrom.SecretKeyRef.Name != expectedSecretName {
			t.Fatalf("POSTGRES_USER secret name: expected %q, got %q",
				expectedSecretName, userEnv.ValueFrom.SecretKeyRef.Name)
		}

		// POSTGRES_PASSWORD sourced from {kb-name}-secrets
		passEnv, exists := envMap["POSTGRES_PASSWORD"]
		if !exists {
			t.Fatal("missing POSTGRES_PASSWORD environment variable")
		}
		if passEnv.ValueFrom == nil || passEnv.ValueFrom.SecretKeyRef == nil {
			t.Fatal("POSTGRES_PASSWORD must be sourced from a Secret")
		}
		if passEnv.ValueFrom.SecretKeyRef.Name != expectedSecretName {
			t.Fatalf("POSTGRES_PASSWORD secret name: expected %q, got %q",
				expectedSecretName, passEnv.ValueFrom.SecretKeyRef.Name)
		}

		// Liveness probe uses pg_isready -U archon
		if container.LivenessProbe == nil {
			t.Fatal("liveness probe is nil")
		}
		if container.LivenessProbe.Exec == nil {
			t.Fatal("liveness probe Exec is nil")
		}
		expectedCmd := []string{"pg_isready", "-U", PostgresDBUser}
		if len(container.LivenessProbe.Exec.Command) != len(expectedCmd) {
			t.Fatalf("liveness probe command length: expected %d, got %d",
				len(expectedCmd), len(container.LivenessProbe.Exec.Command))
		}
		for i, part := range expectedCmd {
			if container.LivenessProbe.Exec.Command[i] != part {
				t.Fatalf("liveness probe command[%d]: expected %q, got %q",
					i, part, container.LivenessProbe.Exec.Command[i])
			}
		}

		// Readiness probe uses pg_isready -U archon
		if container.ReadinessProbe == nil {
			t.Fatal("readiness probe is nil")
		}
		if container.ReadinessProbe.Exec == nil {
			t.Fatal("readiness probe Exec is nil")
		}
		if len(container.ReadinessProbe.Exec.Command) != len(expectedCmd) {
			t.Fatalf("readiness probe command length: expected %d, got %d",
				len(expectedCmd), len(container.ReadinessProbe.Exec.Command))
		}
		for i, part := range expectedCmd {
			if container.ReadinessProbe.Exec.Command[i] != part {
				t.Fatalf("readiness probe command[%d]: expected %q, got %q",
					i, part, container.ReadinessProbe.Exec.Command[i])
			}
		}

		// Service name and type
		if svc.Name != expectedName {
			t.Fatalf("Service name: expected %q, got %q", expectedName, svc.Name)
		}
		if svc.Spec.Type != corev1.ServiceTypeClusterIP {
			t.Fatalf("Service type: expected ClusterIP, got %s", svc.Spec.Type)
		}

		// Service exposes port 5432
		svcPortMap := make(map[int32]bool)
		for _, p := range svc.Spec.Ports {
			svcPortMap[p.Port] = true
		}
		if !svcPortMap[int32(PostgresPort)] {
			t.Fatalf("Service missing port %d", PostgresPort)
		}
	})
}

// Feature: kb-infra-provisioning, Property 4: App ConfigMap Builder Correctness
// **Validates: Requirements 4.1, 4.3**
func TestProperty_AppConfigMapBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		cm := buildAppConfigMap(kb)

		expectedName := fmt.Sprintf("%s-config", kb.Name)

		// 1. Name is {kb-name}-config
		if cm.Name != expectedName {
			t.Fatalf("ConfigMap name: expected %q, got %q", expectedName, cm.Name)
		}

		// 2. Namespace matches KB
		if cm.Namespace != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("ConfigMap namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), cm.Namespace)
		}

		// 3. Has all 8 data keys with correct values
		expectedData := map[string]string{
			"embedding_service_url": fmt.Sprintf("http://%s-embedding-svc:%d", kb.Name, EmbeddingPort),
			"embedding_model":       EmbeddingModel,
			"vector_db_url":         fmt.Sprintf("http://%s-qdrant:%d", kb.Name, QdrantHTTPPort),
			"collection_name":       CollectionName,
			"postgres_host":         fmt.Sprintf("%s-postgres", kb.Name),
			"postgres_port":         fmt.Sprintf("%d", PostgresPort),
			"postgres_db":           PostgresDBName,
			"retrieval_k":           RetrievalK,
		}

		if len(cm.Data) != len(expectedData) {
			t.Fatalf("ConfigMap data key count: expected %d, got %d", len(expectedData), len(cm.Data))
		}

		for key, expectedVal := range expectedData {
			actual, exists := cm.Data[key]
			if !exists {
				t.Fatalf("ConfigMap missing key %q", key)
			}
			if actual != expectedVal {
				t.Fatalf("ConfigMap data[%q]: expected %q, got %q", key, expectedVal, actual)
			}
		}

		// 4. embedding_service_url contains KB name
		if !strings.Contains(cm.Data["embedding_service_url"], kb.Name) {
			t.Fatalf("embedding_service_url %q does not contain KB name %q",
				cm.Data["embedding_service_url"], kb.Name)
		}

		// 5. vector_db_url contains KB name
		if !strings.Contains(cm.Data["vector_db_url"], kb.Name) {
			t.Fatalf("vector_db_url %q does not contain KB name %q",
				cm.Data["vector_db_url"], kb.Name)
		}

		// 6. postgres_host contains KB name
		if !strings.Contains(cm.Data["postgres_host"], kb.Name) {
			t.Fatalf("postgres_host %q does not contain KB name %q",
				cm.Data["postgres_host"], kb.Name)
		}
	})
}

// Feature: kb-infra-provisioning, Property 5: ExternalSecret Builder Correctness
// **Validates: Requirements 5.1, 5.2**
func TestProperty_ExternalSecretBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		obj := buildExternalSecret(kb)

		expectedName := fmt.Sprintf("%s-secrets", kb.Name)
		expectedStoreName := fmt.Sprintf("org-%s-store", kb.Spec.Organization)

		// 1. Name is {kb-name}-secrets
		if obj.GetName() != expectedName {
			t.Fatalf("ExternalSecret name: expected %q, got %q", expectedName, obj.GetName())
		}

		// 2. Namespace matches KB
		if obj.GetNamespace() != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("ExternalSecret namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), obj.GetNamespace())
		}

		// 3. GVK is external-secrets.io/v1/ExternalSecret
		gvk := obj.GroupVersionKind()
		if gvk.Group != "external-secrets.io" || gvk.Version != "v1" || gvk.Kind != "ExternalSecret" {
			t.Fatalf("ExternalSecret GVK: expected external-secrets.io/v1/ExternalSecret, got %s/%s/%s",
				gvk.Group, gvk.Version, gvk.Kind)
		}

		spec, ok := obj.Object["spec"].(map[string]interface{})
		if !ok {
			t.Fatal("ExternalSecret spec is missing or not a map")
		}

		// 4. spec.secretStoreRef.name is org-{organization}-store
		storeRef, ok := spec["secretStoreRef"].(map[string]interface{})
		if !ok {
			t.Fatal("spec.secretStoreRef is missing or not a map")
		}
		if storeRef["name"] != expectedStoreName {
			t.Fatalf("secretStoreRef.name: expected %q, got %q", expectedStoreName, storeRef["name"])
		}

		// 5. spec.secretStoreRef.kind is ClusterSecretStore
		if storeRef["kind"] != "ClusterSecretStore" {
			t.Fatalf("secretStoreRef.kind: expected %q, got %q", "ClusterSecretStore", storeRef["kind"])
		}

		// 6. spec.target.name is {kb-name}-secrets
		target, ok := spec["target"].(map[string]interface{})
		if !ok {
			t.Fatal("spec.target is missing or not a map")
		}
		if target["name"] != expectedName {
			t.Fatalf("target.name: expected %q, got %q", expectedName, target["name"])
		}

		// 7. spec.target.creationPolicy is Owner
		if target["creationPolicy"] != "Owner" {
			t.Fatalf("target.creationPolicy: expected %q, got %q", "Owner", target["creationPolicy"])
		}

		// 8. spec.data has 3 entries mapping github_token, postgres_user, postgres_password
		data, ok := spec["data"].([]interface{})
		if !ok {
			t.Fatal("spec.data is missing or not a slice")
		}
		if len(data) != 3 {
			t.Fatalf("spec.data length: expected 3, got %d", len(data))
		}

		expectedSecretKeys := map[string]bool{
			"github_token":      false,
			"postgres_user":     false,
			"postgres_password": false,
		}

		for _, entry := range data {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				t.Fatal("spec.data entry is not a map")
			}

			secretKey, ok := entryMap["secretKey"].(string)
			if !ok {
				t.Fatal("spec.data entry missing secretKey")
			}

			if _, expected := expectedSecretKeys[secretKey]; !expected {
				t.Fatalf("unexpected secretKey %q in spec.data", secretKey)
			}
			expectedSecretKeys[secretKey] = true

			// 9. All remote refs use org-secrets key
			remoteRef, ok := entryMap["remoteRef"].(map[string]interface{})
			if !ok {
				t.Fatalf("spec.data entry %q missing remoteRef", secretKey)
			}
			if remoteRef["key"] != OrgSecretsRemoteKey {
				t.Fatalf("remoteRef.key for %q: expected %q, got %q",
					secretKey, OrgSecretsRemoteKey, remoteRef["key"])
			}
		}

		for key, found := range expectedSecretKeys {
			if !found {
				t.Fatalf("spec.data missing entry for secretKey %q", key)
			}
		}
	})
}

// Feature: kb-infra-provisioning, Property 6: Embedding Builder Correctness
// **Validates: Requirements 6.1, 6.2, 6.3**
func TestProperty_EmbeddingBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		deploy := buildEmbeddingDeployment(kb)
		svc := buildEmbeddingService(kb)

		expectedDeployName := fmt.Sprintf("%s-embedding", kb.Name)
		expectedSvcName := fmt.Sprintf("%s-embedding-svc", kb.Name)
		expectedConfigMapName := fmt.Sprintf("%s-config", kb.Name)

		// 1. Deployment name is {kb-name}-embedding
		if deploy.Name != expectedDeployName {
			t.Fatalf("Deployment name: expected %q, got %q", expectedDeployName, deploy.Name)
		}

		// 2. Deployment namespace matches KB namespace
		if deploy.Namespace != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("Deployment namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), deploy.Namespace)
		}

		// 3. Container image is EmbeddingImage
		container := deploy.Spec.Template.Spec.Containers[0]
		if container.Image != EmbeddingImage {
			t.Fatalf("container image: expected %q, got %q", EmbeddingImage, container.Image)
		}

		// 4. Container port 8000
		portMap := make(map[int32]bool)
		for _, p := range container.Ports {
			portMap[p.ContainerPort] = true
		}
		if !portMap[int32(EmbeddingPort)] {
			t.Fatalf("container missing port %d", EmbeddingPort)
		}

		// 5. Memory request 2Gi, limit 4Gi
		memReq := container.Resources.Requests[corev1.ResourceMemory]
		expectedMemReq := mustParseQuantity(EmbeddingMemoryRequest)
		if memReq.Cmp(expectedMemReq) != 0 {
			t.Fatalf("memory request: expected %s, got %s", expectedMemReq.String(), memReq.String())
		}
		memLimit := container.Resources.Limits[corev1.ResourceMemory]
		expectedMemLimit := mustParseQuantity(EmbeddingMemoryLimit)
		if memLimit.Cmp(expectedMemLimit) != 0 {
			t.Fatalf("memory limit: expected %s, got %s", expectedMemLimit.String(), memLimit.String())
		}

		// 6. CPU request 500m, limit 2000m
		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		expectedCPUReq := mustParseQuantity(EmbeddingCPURequest)
		if cpuReq.Cmp(expectedCPUReq) != 0 {
			t.Fatalf("CPU request: expected %s, got %s", expectedCPUReq.String(), cpuReq.String())
		}
		cpuLimit := container.Resources.Limits[corev1.ResourceCPU]
		expectedCPULimit := mustParseQuantity(EmbeddingCPULimit)
		if cpuLimit.Cmp(expectedCPULimit) != 0 {
			t.Fatalf("CPU limit: expected %s, got %s", expectedCPULimit.String(), cpuLimit.String())
		}

		// 7. EMBEDDING_MODEL env var sourced from {kb-name}-config ConfigMap key embedding_model
		envMap := make(map[string]corev1.EnvVar)
		for _, env := range container.Env {
			envMap[env.Name] = env
		}
		modelEnv, exists := envMap["EMBEDDING_MODEL"]
		if !exists {
			t.Fatal("missing EMBEDDING_MODEL environment variable")
		}
		if modelEnv.ValueFrom == nil || modelEnv.ValueFrom.ConfigMapKeyRef == nil {
			t.Fatal("EMBEDDING_MODEL must be sourced from a ConfigMap")
		}
		if modelEnv.ValueFrom.ConfigMapKeyRef.Name != expectedConfigMapName {
			t.Fatalf("EMBEDDING_MODEL ConfigMap name: expected %q, got %q",
				expectedConfigMapName, modelEnv.ValueFrom.ConfigMapKeyRef.Name)
		}
		if modelEnv.ValueFrom.ConfigMapKeyRef.Key != "embedding_model" {
			t.Fatalf("EMBEDDING_MODEL ConfigMap key: expected %q, got %q",
				"embedding_model", modelEnv.ValueFrom.ConfigMapKeyRef.Key)
		}

		// 8. Liveness probe on /health port 8000 with initialDelaySeconds 30, periodSeconds 30
		if container.LivenessProbe == nil {
			t.Fatal("liveness probe is nil")
		}
		if container.LivenessProbe.HTTPGet == nil {
			t.Fatal("liveness probe HTTPGet is nil")
		}
		if container.LivenessProbe.HTTPGet.Path != "/health" {
			t.Fatalf("liveness probe path: expected %q, got %q", "/health", container.LivenessProbe.HTTPGet.Path)
		}
		livenessPort := container.LivenessProbe.HTTPGet.Port.IntValue()
		if livenessPort != EmbeddingPort {
			t.Fatalf("liveness probe port: expected %d, got %d", EmbeddingPort, livenessPort)
		}
		if container.LivenessProbe.InitialDelaySeconds != 30 {
			t.Fatalf("liveness probe initialDelaySeconds: expected 30, got %d", container.LivenessProbe.InitialDelaySeconds)
		}
		if container.LivenessProbe.PeriodSeconds != 30 {
			t.Fatalf("liveness probe periodSeconds: expected 30, got %d", container.LivenessProbe.PeriodSeconds)
		}

		// 9. Readiness probe on /ready port 8000 with initialDelaySeconds 60, periodSeconds 10
		if container.ReadinessProbe == nil {
			t.Fatal("readiness probe is nil")
		}
		if container.ReadinessProbe.HTTPGet == nil {
			t.Fatal("readiness probe HTTPGet is nil")
		}
		if container.ReadinessProbe.HTTPGet.Path != "/ready" {
			t.Fatalf("readiness probe path: expected %q, got %q", "/ready", container.ReadinessProbe.HTTPGet.Path)
		}
		readinessPort := container.ReadinessProbe.HTTPGet.Port.IntValue()
		if readinessPort != EmbeddingPort {
			t.Fatalf("readiness probe port: expected %d, got %d", EmbeddingPort, readinessPort)
		}
		if container.ReadinessProbe.InitialDelaySeconds != 60 {
			t.Fatalf("readiness probe initialDelaySeconds: expected 60, got %d", container.ReadinessProbe.InitialDelaySeconds)
		}
		if container.ReadinessProbe.PeriodSeconds != 10 {
			t.Fatalf("readiness probe periodSeconds: expected 10, got %d", container.ReadinessProbe.PeriodSeconds)
		}

		// 10. Service name is {kb-name}-embedding-svc
		if svc.Name != expectedSvcName {
			t.Fatalf("Service name: expected %q, got %q", expectedSvcName, svc.Name)
		}

		// 11. Service type is ClusterIP
		if svc.Spec.Type != corev1.ServiceTypeClusterIP {
			t.Fatalf("Service type: expected ClusterIP, got %s", svc.Spec.Type)
		}

		// 12. Service exposes port 8000
		svcPortMap := make(map[int32]bool)
		for _, p := range svc.Spec.Ports {
			svcPortMap[p.Port] = true
		}
		if !svcPortMap[int32(EmbeddingPort)] {
			t.Fatalf("Service missing port %d", EmbeddingPort)
		}
	})
}

// Feature: kb-infra-provisioning, Property 7: Query Builder Correctness
// **Validates: Requirements 7.1, 7.2, 7.3**
func TestProperty_QueryBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		deploy := buildQueryDeployment(kb)
		svc := buildQueryService(kb)

		expectedDeployName := fmt.Sprintf("%s-query", kb.Name)
		expectedSvcName := fmt.Sprintf("%s-query", kb.Name)
		expectedConfigMapName := fmt.Sprintf("%s-config", kb.Name)

		// 1. Deployment name is {kb-name}-query
		if deploy.Name != expectedDeployName {
			t.Fatalf("Deployment name: expected %q, got %q", expectedDeployName, deploy.Name)
		}

		// 2. Deployment namespace matches KB namespace
		if deploy.Namespace != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("Deployment namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), deploy.Namespace)
		}

		// 3. Container image is QueryImage
		container := deploy.Spec.Template.Spec.Containers[0]
		if container.Image != QueryImage {
			t.Fatalf("container image: expected %q, got %q", QueryImage, container.Image)
		}

		// 4. Container port 8080
		portMap := make(map[int32]bool)
		for _, p := range container.Ports {
			portMap[p.ContainerPort] = true
		}
		if !portMap[int32(QueryPort)] {
			t.Fatalf("container missing port %d", QueryPort)
		}

		// 5. Replicas is 2 (QueryReplicas)
		if deploy.Spec.Replicas == nil {
			t.Fatal("replicas is nil")
		}
		if *deploy.Spec.Replicas != int32(QueryReplicas) {
			t.Fatalf("replicas: expected %d, got %d", QueryReplicas, *deploy.Spec.Replicas)
		}

		// 6. Memory request 256Mi, limit 512Mi
		memReq := container.Resources.Requests[corev1.ResourceMemory]
		expectedMemReq := mustParseQuantity(QueryMemoryRequest)
		if memReq.Cmp(expectedMemReq) != 0 {
			t.Fatalf("memory request: expected %s, got %s", expectedMemReq.String(), memReq.String())
		}
		memLimit := container.Resources.Limits[corev1.ResourceMemory]
		expectedMemLimit := mustParseQuantity(QueryMemoryLimit)
		if memLimit.Cmp(expectedMemLimit) != 0 {
			t.Fatalf("memory limit: expected %s, got %s", expectedMemLimit.String(), memLimit.String())
		}

		// 7. CPU request 100m, limit 500m
		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		expectedCPUReq := mustParseQuantity(QueryCPURequest)
		if cpuReq.Cmp(expectedCPUReq) != 0 {
			t.Fatalf("CPU request: expected %s, got %s", expectedCPUReq.String(), cpuReq.String())
		}
		cpuLimit := container.Resources.Limits[corev1.ResourceCPU]
		expectedCPULimit := mustParseQuantity(QueryCPULimit)
		if cpuLimit.Cmp(expectedCPULimit) != 0 {
			t.Fatalf("CPU limit: expected %s, got %s", expectedCPULimit.String(), cpuLimit.String())
		}

		// 8. Five env vars all sourced from {kb-name}-config ConfigMap
		envMap := make(map[string]corev1.EnvVar)
		for _, env := range container.Env {
			envMap[env.Name] = env
		}

		expectedEnvVars := []struct {
			envName      string
			configMapKey string
		}{
			{"EMBEDDING_SERVICE_URL", "embedding_service_url"},
			{"EMBEDDING_MODEL", "embedding_model"},
			{"VECTOR_DB_URL", "vector_db_url"},
			{"COLLECTION_NAME", "collection_name"},
			{"RETRIEVAL_K", "retrieval_k"},
		}

		for _, expected := range expectedEnvVars {
			env, exists := envMap[expected.envName]
			if !exists {
				t.Fatalf("missing %s environment variable", expected.envName)
			}
			if env.ValueFrom == nil || env.ValueFrom.ConfigMapKeyRef == nil {
				t.Fatalf("%s must be sourced from a ConfigMap", expected.envName)
			}
			if env.ValueFrom.ConfigMapKeyRef.Name != expectedConfigMapName {
				t.Fatalf("%s ConfigMap name: expected %q, got %q",
					expected.envName, expectedConfigMapName, env.ValueFrom.ConfigMapKeyRef.Name)
			}
			if env.ValueFrom.ConfigMapKeyRef.Key != expected.configMapKey {
				t.Fatalf("%s ConfigMap key: expected %q, got %q",
					expected.envName, expected.configMapKey, env.ValueFrom.ConfigMapKeyRef.Key)
			}
		}

		if len(container.Env) != len(expectedEnvVars) {
			t.Fatalf("expected %d env vars, got %d", len(expectedEnvVars), len(container.Env))
		}

		// 9. Liveness probe on /health port 8080
		if container.LivenessProbe == nil {
			t.Fatal("liveness probe is nil")
		}
		if container.LivenessProbe.HTTPGet == nil {
			t.Fatal("liveness probe HTTPGet is nil")
		}
		if container.LivenessProbe.HTTPGet.Path != "/health" {
			t.Fatalf("liveness probe path: expected %q, got %q", "/health", container.LivenessProbe.HTTPGet.Path)
		}
		livenessPort := container.LivenessProbe.HTTPGet.Port.IntValue()
		if livenessPort != QueryPort {
			t.Fatalf("liveness probe port: expected %d, got %d", QueryPort, livenessPort)
		}

		// 10. Readiness probe on /ready port 8080
		if container.ReadinessProbe == nil {
			t.Fatal("readiness probe is nil")
		}
		if container.ReadinessProbe.HTTPGet == nil {
			t.Fatal("readiness probe HTTPGet is nil")
		}
		if container.ReadinessProbe.HTTPGet.Path != "/ready" {
			t.Fatalf("readiness probe path: expected %q, got %q", "/ready", container.ReadinessProbe.HTTPGet.Path)
		}
		readinessPort := container.ReadinessProbe.HTTPGet.Port.IntValue()
		if readinessPort != QueryPort {
			t.Fatalf("readiness probe port: expected %d, got %d", QueryPort, readinessPort)
		}

		// 11. Service name is {kb-name}-query
		if svc.Name != expectedSvcName {
			t.Fatalf("Service name: expected %q, got %q", expectedSvcName, svc.Name)
		}

		// 12. Service type is ClusterIP
		if svc.Spec.Type != corev1.ServiceTypeClusterIP {
			t.Fatalf("Service type: expected ClusterIP, got %s", svc.Spec.Type)
		}

		// 13. Service exposes port 8080
		svcPortMap := make(map[int32]bool)
		for _, p := range svc.Spec.Ports {
			svcPortMap[p.Port] = true
		}
		if !svcPortMap[int32(QueryPort)] {
			t.Fatalf("Service missing port %d", QueryPort)
		}
	})
}

// Feature: kb-infra-provisioning, Property 8: HTTPRoute Builder Correctness
// **Validates: Requirements 8.1, 8.2**
func TestProperty_HTTPRouteBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		obj := buildHTTPRoute(kb)

		expectedName := fmt.Sprintf("%s-route", kb.Name)
		expectedQuerySvc := fmt.Sprintf("%s-query", kb.Name)

		// 1. Name is {kb-name}-route
		if obj.GetName() != expectedName {
			t.Fatalf("HTTPRoute name: expected %q, got %q", expectedName, obj.GetName())
		}

		// 2. Namespace matches KB namespace
		if obj.GetNamespace() != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("HTTPRoute namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), obj.GetNamespace())
		}

		// 3. GVK is gateway.networking.k8s.io/v1/HTTPRoute
		gvk := obj.GroupVersionKind()
		if gvk.Group != "gateway.networking.k8s.io" || gvk.Version != "v1" || gvk.Kind != "HTTPRoute" {
			t.Fatalf("HTTPRoute GVK: expected gateway.networking.k8s.io/v1/HTTPRoute, got %s/%s/%s",
				gvk.Group, gvk.Version, gvk.Kind)
		}

		spec, ok := obj.Object["spec"].(map[string]interface{})
		if !ok {
			t.Fatal("spec is missing or not a map")
		}

		// 4-6. parentRefs
		parentRefs, ok := spec["parentRefs"].([]interface{})
		if !ok {
			t.Fatal("spec.parentRefs is missing or not a slice")
		}
		if len(parentRefs) != 1 {
			t.Fatalf("spec.parentRefs length: expected 1, got %d", len(parentRefs))
		}
		parentRef, ok := parentRefs[0].(map[string]interface{})
		if !ok {
			t.Fatal("spec.parentRefs[0] is not a map")
		}

		// 4. parentRefs[0].name is platform-gateway
		if parentRef["name"] != PlatformGatewayName {
			t.Fatalf("parentRef.name: expected %q, got %q", PlatformGatewayName, parentRef["name"])
		}

		// 5. parentRefs[0].namespace is kube-system
		if parentRef["namespace"] != PlatformGatewayNamespace {
			t.Fatalf("parentRef.namespace: expected %q, got %q", PlatformGatewayNamespace, parentRef["namespace"])
		}

		// 6. parentRefs[0].sectionName is https
		if parentRef["sectionName"] != PlatformGatewaySectionName {
			t.Fatalf("parentRef.sectionName: expected %q, got %q", PlatformGatewaySectionName, parentRef["sectionName"])
		}

		// 7. hostnames[0] contains the KB name
		hostnames, ok := spec["hostnames"].([]interface{})
		if !ok {
			t.Fatal("spec.hostnames is missing or not a slice")
		}
		if len(hostnames) < 1 {
			t.Fatal("spec.hostnames is empty")
		}
		hostname, ok := hostnames[0].(string)
		if !ok {
			t.Fatal("spec.hostnames[0] is not a string")
		}
		if !strings.Contains(hostname, kb.Name) {
			t.Fatalf("hostname %q does not contain KB name %q", hostname, kb.Name)
		}

		// 8-11. rules
		rules, ok := spec["rules"].([]interface{})
		if !ok {
			t.Fatal("spec.rules is missing or not a slice")
		}
		if len(rules) < 1 {
			t.Fatal("spec.rules is empty")
		}
		rule, ok := rules[0].(map[string]interface{})
		if !ok {
			t.Fatal("spec.rules[0] is not a map")
		}

		// 8-9. matches[0].path.type is PathPrefix, value is /
		matches, ok := rule["matches"].([]interface{})
		if !ok {
			t.Fatal("spec.rules[0].matches is missing or not a slice")
		}
		if len(matches) < 1 {
			t.Fatal("spec.rules[0].matches is empty")
		}
		match, ok := matches[0].(map[string]interface{})
		if !ok {
			t.Fatal("spec.rules[0].matches[0] is not a map")
		}
		path, ok := match["path"].(map[string]interface{})
		if !ok {
			t.Fatal("spec.rules[0].matches[0].path is missing or not a map")
		}

		// 8. path.type is PathPrefix
		if path["type"] != "PathPrefix" {
			t.Fatalf("path.type: expected %q, got %q", "PathPrefix", path["type"])
		}

		// 9. path.value is /
		if path["value"] != "/" {
			t.Fatalf("path.value: expected %q, got %q", "/", path["value"])
		}

		// 10-11. backendRefs
		backendRefs, ok := rule["backendRefs"].([]interface{})
		if !ok {
			t.Fatal("spec.rules[0].backendRefs is missing or not a slice")
		}
		if len(backendRefs) < 1 {
			t.Fatal("spec.rules[0].backendRefs is empty")
		}
		backendRef, ok := backendRefs[0].(map[string]interface{})
		if !ok {
			t.Fatal("spec.rules[0].backendRefs[0] is not a map")
		}

		// 10. backendRefs[0].name is {kb-name}-query
		if backendRef["name"] != expectedQuerySvc {
			t.Fatalf("backendRef.name: expected %q, got %q", expectedQuerySvc, backendRef["name"])
		}

		// 11. backendRefs[0].port is 8080
		port, ok := backendRef["port"].(int64)
		if !ok {
			t.Fatalf("backendRef.port is not int64, got %T", backendRef["port"])
		}
		if port != int64(QueryPort) {
			t.Fatalf("backendRef.port: expected %d, got %d", QueryPort, port)
		}
	})
}

// Feature: graph-service-deployment, Property 1: Graph Builder Correctness
// **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 2.10, 3.1, 3.2, 3.3, 5.2, 5.3**
func TestProperty_GraphBuilderCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		kb := knowledgeBaseGenerator().Draw(t, "knowledgeBase")

		deploy := buildGraphDeployment(kb)
		svc := buildGraphService(kb)

		expectedDeployName := fmt.Sprintf("%s-graph", kb.Name)
		expectedSvcName := fmt.Sprintf("%s-graph", kb.Name)
		expectedConfigMapName := fmt.Sprintf("%s-config", kb.Name)
		expectedSecretName := fmt.Sprintf("%s-secrets", kb.Name)

		// 1. Deployment name is {kb-name}-graph (Req 2.1)
		if deploy.Name != expectedDeployName {
			t.Fatalf("Deployment name: expected %q, got %q", expectedDeployName, deploy.Name)
		}

		// 2. Deployment namespace matches KB namespace (Req 2.1)
		if deploy.Namespace != fmt.Sprintf("kb-%s", kb.Name) {
			t.Fatalf("Deployment namespace: expected %q, got %q", fmt.Sprintf("kb-%s", kb.Name), deploy.Namespace)
		}

		// 3. Container image is GraphImage (Req 2.2)
		container := deploy.Spec.Template.Spec.Containers[0]
		if container.Image != GraphImage {
			t.Fatalf("container image: expected %q, got %q", GraphImage, container.Image)
		}

		// 4. Container port 8081 (Req 2.3)
		portMap := make(map[int32]bool)
		for _, p := range container.Ports {
			portMap[p.ContainerPort] = true
		}
		if !portMap[int32(GraphPort)] {
			t.Fatalf("container missing port %d", GraphPort)
		}

		// 5. Replicas is 1 (GraphReplicas) (Req 2.4)
		if deploy.Spec.Replicas == nil {
			t.Fatal("replicas is nil")
		}
		if *deploy.Spec.Replicas != int32(GraphReplicas) {
			t.Fatalf("replicas: expected %d, got %d", GraphReplicas, *deploy.Spec.Replicas)
		}

		// 6. Memory request 256Mi, limit 512Mi (Req 2.5)
		memReq := container.Resources.Requests[corev1.ResourceMemory]
		expectedMemReq := mustParseQuantity(GraphMemoryRequest)
		if memReq.Cmp(expectedMemReq) != 0 {
			t.Fatalf("memory request: expected %s, got %s", expectedMemReq.String(), memReq.String())
		}
		memLimit := container.Resources.Limits[corev1.ResourceMemory]
		expectedMemLimit := mustParseQuantity(GraphMemoryLimit)
		if memLimit.Cmp(expectedMemLimit) != 0 {
			t.Fatalf("memory limit: expected %s, got %s", expectedMemLimit.String(), memLimit.String())
		}

		// 7. CPU request 100m, limit 500m (Req 2.6)
		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		expectedCPUReq := mustParseQuantity(GraphCPURequest)
		if cpuReq.Cmp(expectedCPUReq) != 0 {
			t.Fatalf("CPU request: expected %s, got %s", expectedCPUReq.String(), cpuReq.String())
		}
		cpuLimit := container.Resources.Limits[corev1.ResourceCPU]
		expectedCPULimit := mustParseQuantity(GraphCPULimit)
		if cpuLimit.Cmp(expectedCPULimit) != 0 {
			t.Fatalf("CPU limit: expected %s, got %s", expectedCPULimit.String(), cpuLimit.String())
		}

		// 8. Liveness probe on /health port 8081 (Req 2.7)
		if container.LivenessProbe == nil {
			t.Fatal("liveness probe is nil")
		}
		if container.LivenessProbe.HTTPGet == nil {
			t.Fatal("liveness probe HTTPGet is nil")
		}
		if container.LivenessProbe.HTTPGet.Path != "/health" {
			t.Fatalf("liveness probe path: expected %q, got %q", "/health", container.LivenessProbe.HTTPGet.Path)
		}
		livenessPort := container.LivenessProbe.HTTPGet.Port.IntValue()
		if livenessPort != GraphPort {
			t.Fatalf("liveness probe port: expected %d, got %d", GraphPort, livenessPort)
		}

		// 9. Readiness probe on /ready port 8081 (Req 2.8)
		if container.ReadinessProbe == nil {
			t.Fatal("readiness probe is nil")
		}
		if container.ReadinessProbe.HTTPGet == nil {
			t.Fatal("readiness probe HTTPGet is nil")
		}
		if container.ReadinessProbe.HTTPGet.Path != "/ready" {
			t.Fatalf("readiness probe path: expected %q, got %q", "/ready", container.ReadinessProbe.HTTPGet.Path)
		}
		readinessPort := container.ReadinessProbe.HTTPGet.Port.IntValue()
		if readinessPort != GraphPort {
			t.Fatalf("readiness probe port: expected %d, got %d", GraphPort, readinessPort)
		}

		// 10. Three env vars from ConfigMap, two from Secret (Req 2.9, 2.10)
		envMap := make(map[string]corev1.EnvVar)
		for _, env := range container.Env {
			envMap[env.Name] = env
		}

		expectedConfigMapEnvVars := []struct {
			envName      string
			configMapKey string
		}{
			{"POSTGRES_HOST", "postgres_host"},
			{"POSTGRES_PORT", "postgres_port"},
			{"POSTGRES_DB", "postgres_db"},
		}

		for _, expected := range expectedConfigMapEnvVars {
			env, exists := envMap[expected.envName]
			if !exists {
				t.Fatalf("missing %s environment variable", expected.envName)
			}
			if env.ValueFrom == nil || env.ValueFrom.ConfigMapKeyRef == nil {
				t.Fatalf("%s must be sourced from a ConfigMap", expected.envName)
			}
			if env.ValueFrom.ConfigMapKeyRef.Name != expectedConfigMapName {
				t.Fatalf("%s ConfigMap name: expected %q, got %q",
					expected.envName, expectedConfigMapName, env.ValueFrom.ConfigMapKeyRef.Name)
			}
			if env.ValueFrom.ConfigMapKeyRef.Key != expected.configMapKey {
				t.Fatalf("%s ConfigMap key: expected %q, got %q",
					expected.envName, expected.configMapKey, env.ValueFrom.ConfigMapKeyRef.Key)
			}
		}

		expectedSecretEnvVars := []struct {
			envName   string
			secretKey string
		}{
			{"POSTGRES_USER", "postgres_user"},
			{"POSTGRES_PASSWORD", "postgres_password"},
		}

		for _, expected := range expectedSecretEnvVars {
			env, exists := envMap[expected.envName]
			if !exists {
				t.Fatalf("missing %s environment variable", expected.envName)
			}
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
				t.Fatalf("%s must be sourced from a Secret", expected.envName)
			}
			if env.ValueFrom.SecretKeyRef.Name != expectedSecretName {
				t.Fatalf("%s Secret name: expected %q, got %q",
					expected.envName, expectedSecretName, env.ValueFrom.SecretKeyRef.Name)
			}
			if env.ValueFrom.SecretKeyRef.Key != expected.secretKey {
				t.Fatalf("%s Secret key: expected %q, got %q",
					expected.envName, expected.secretKey, env.ValueFrom.SecretKeyRef.Key)
			}
		}

		if len(container.Env) != 5 {
			t.Fatalf("expected 5 env vars (3 ConfigMap + 2 Secret), got %d", len(container.Env))
		}

		// 11. Labels include app.kubernetes.io/component = "graph" (Req 5.3)
		if deploy.Labels["app.kubernetes.io/component"] != "graph" {
			t.Fatalf("Deployment component label: expected %q, got %q", "graph", deploy.Labels["app.kubernetes.io/component"])
		}

		// 12. Service name is {kb-name}-graph (Req 3.1)
		if svc.Name != expectedSvcName {
			t.Fatalf("Service name: expected %q, got %q", expectedSvcName, svc.Name)
		}

		// 13. Service type is ClusterIP (Req 3.2)
		if svc.Spec.Type != corev1.ServiceTypeClusterIP {
			t.Fatalf("Service type: expected ClusterIP, got %s", svc.Spec.Type)
		}

		// 14. Service exposes port 8081 (Req 3.3)
		svcPortMap := make(map[int32]bool)
		for _, p := range svc.Spec.Ports {
			svcPortMap[p.Port] = true
		}
		if !svcPortMap[int32(GraphPort)] {
			t.Fatalf("Service missing port %d", GraphPort)
		}

		// 15. graphDeploymentName returns {kb-name}-graph (Req 5.2)
		if graphDeploymentName(kb) != expectedDeployName {
			t.Fatalf("graphDeploymentName: expected %q, got %q", expectedDeployName, graphDeploymentName(kb))
		}

		// 16. graphServiceName returns {kb-name}-graph (Req 5.2)
		if graphServiceName(kb) != expectedSvcName {
			t.Fatalf("graphServiceName: expected %q, got %q", expectedSvcName, graphServiceName(kb))
		}
	})
}
