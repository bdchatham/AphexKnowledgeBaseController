# Design: SCIP Knowledge Integration for KnowledgeBase CRD

## Overview

This design extends the existing KnowledgeBase CRD and controller to support SCIP-driven knowledge configuration. The changes are additive — all existing functionality (repository tracking, MCP server provisioning) remains unchanged.

**Parent Spec:** `ArchonAgent/.kiro/specs/agent-orchestration/` (Requirement 13)

## CRD Schema Changes

### New Spec Fields

Added to `KnowledgeBaseSpec` in `AphexControllerRuntime/api/v1alpha1/knowledgebase_types.go`:

```go
// SCIPIndexingConfig defines SCIP code analysis configuration
type SCIPIndexingConfig struct {
    // Enabled controls whether SCIP indexing runs for this KnowledgeBase
    // +kubebuilder:default:=false
    // +optional
    Enabled bool `json:"enabled,omitempty"`

    // Languages is the list of languages to index
    // +kubebuilder:validation:Items:Enum=go;typescript;python;java
    // +optional
    Languages []string `json:"languages,omitempty"`
}

// VectorStoreConfig defines vector store configuration
type VectorStoreConfig struct {
    // Source determines which documentation files to embed
    // +kubebuilder:validation:Enum=kiro-docs;archon-docs
    // +kubebuilder:default:="archon-docs"
    // +optional
    Source string `json:"source,omitempty"`

    // EmbeddingModel is the model used for generating embeddings
    // +kubebuilder:default:="BAAI/bge-base-en-v1.5"
    // +optional
    EmbeddingModel string `json:"embeddingModel,omitempty"`

    // CollectionName is the Qdrant collection name
    // +optional
    CollectionName string `json:"collectionName,omitempty"`
}

// CodeGraphConfig defines code graph configuration
type CodeGraphConfig struct {
    // Enabled controls whether code graph queries are available
    // +kubebuilder:default:=false
    // +optional
    Enabled bool `json:"enabled,omitempty"`

    // GraphQLEndpoint is the URL of the code graph GraphQL API
    // Required when enabled is true
    // +optional
    GraphQLEndpoint string `json:"graphqlEndpoint,omitempty"`
}
```

Added to `KnowledgeBaseSpec`:

```go
type KnowledgeBaseSpec struct {
    // ... existing fields ...

    // SCIPIndexing configures SCIP-based code analysis
    // +optional
    SCIPIndexing *SCIPIndexingConfig `json:"scipIndexing,omitempty"`

    // VectorStore configures the vector store for semantic search
    // +optional
    VectorStore *VectorStoreConfig `json:"vectorStore,omitempty"`

    // CodeGraph configures the code graph for structural queries
    // +optional
    CodeGraph *CodeGraphConfig `json:"codeGraph,omitempty"`
}
```

### New Status Fields

Added to `KnowledgeBaseStatus`:

```go
type KnowledgeBaseStatus struct {
    // ... existing fields ...

    // VectorStoreReady indicates the vector store is accessible and populated
    // +optional
    VectorStoreReady bool `json:"vectorStoreReady,omitempty"`

    // CodeGraphReady indicates the code graph endpoint is healthy
    // +optional
    CodeGraphReady bool `json:"codeGraphReady,omitempty"`

    // LastSyncTime is the timestamp of the last successful sync cycle
    // +optional
    LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}
```

## Controller Changes

### Reconciliation Flow

The existing reconciliation loop in `knowledgebase_controller.go` gains two new steps after `reconcileRepositoryConfig` and before `reconcileMCPServer`:

```
validateSpec → ensureFinalizer → reconcileRepositoryConfig
  → reconcileKnowledgeConfig (NEW)
  → reconcileMCPServer → updateStatus
```

### reconcileKnowledgeConfig

Stores SCIP, vector store, and code graph configuration in the existing repository ConfigMap (or a dedicated ConfigMap):

```go
func (r *KnowledgeBaseReconciler) reconcileKnowledgeConfig(
    ctx context.Context, 
    kb *platformv1alpha1.KnowledgeBase,
) error {
    // Add SCIP, vector store, and code graph config to ConfigMap
    // Check vector store health → set status.vectorStoreReady
    // Check code graph health → set status.codeGraphReady
    // Update status.lastSyncTime on success
}
```

### Validation Additions

`validateSpec` gains validation for the new fields:
- If `codeGraph.enabled` is true, `graphqlEndpoint` must be non-empty
- `scipIndexing.languages` values must be in the supported set
- `vectorStore.source` must be a valid enum value

### Status Update Additions

The final status patch includes the new readiness fields:

```go
statusPatch["vectorStoreReady"] = vectorStoreHealthy
statusPatch["codeGraphReady"] = codeGraphHealthy
statusPatch["lastSyncTime"] = metav1.Now().Format(time.RFC3339)
```

## Printer Column Additions

Add printer columns for visibility:

```go
// +kubebuilder:printcolumn:name="Vector Store",type=boolean,JSONPath=`.status.vectorStoreReady`
// +kubebuilder:printcolumn:name="Code Graph",type=boolean,JSONPath=`.status.codeGraphReady`
```

## Example Resource

```yaml
apiVersion: aphex.io/v1alpha1
kind: KnowledgeBase
metadata:
  name: archon-workspace
  namespace: archon-system
spec:
  displayName: "Archon Workspace Knowledge"
  repositories:
    - url: https://github.com/org/ArchonAgent
      branch: main
    - url: https://github.com/org/ArchonDocumentationMCPTools
      branch: main
  scipIndexing:
    enabled: true
    languages: [go, typescript, python]
  vectorStore:
    source: archon-docs
    embeddingModel: BAAI/bge-base-en-v1.5
  codeGraph:
    enabled: true
    graphqlEndpoint: http://code-graph.archon-system:8080/graphql
  mcp:
    image: archon-mcp-server:latest
    port: 8080
status:
  phase: Ready
  vectorStoreReady: true
  codeGraphReady: true
  lastSyncTime: "2026-02-06T12:00:00Z"
  mcp:
    deployed: true
    serviceName: mcp-server-archon-workspace
    serviceURL: http://mcp-server-archon-workspace.archon-system:8080
```

## Migration Strategy

All new fields are optional with sensible defaults. Existing KnowledgeBase resources continue to work without modification. The controller skips SCIP/vector/graph reconciliation when the fields are absent.

## Affected Packages

- **AphexControllerRuntime**: CRD type definitions, deep copy generation, CRD YAML regeneration
- **AphexKnowledgeBaseController**: Controller reconciliation logic, validation, status updates

**Source:**
- `AphexControllerRuntime/api/v1alpha1/knowledgebase_types.go`
- `AphexControllerRuntime/crds/aphex.io_knowledgebases.yaml`
- `AphexKnowledgeBaseController/controller/knowledgebase_controller.go`
