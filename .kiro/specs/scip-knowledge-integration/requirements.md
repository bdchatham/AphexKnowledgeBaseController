# Requirements: SCIP Knowledge Integration for KnowledgeBase CRD

## Introduction

This spec extends the KnowledgeBase CRD to support SCIP-driven knowledge configuration, enabling a two-layer knowledge system (vector store + code graph) that can be declaratively managed through Kubernetes resources.

**Parent Spec Reference:** `ArchonAgent/.kiro/specs/agent-orchestration/` (Requirement 13: KnowledgeBase CRD Evolution)

### Context

The current KnowledgeBase CRD supports:
- Repository tracking with configurable paths and branches
- Optional MCP server provisioning

The agent orchestration pipeline (Phase 3 of Archon Agent) requires the KnowledgeBase to also manage:
- SCIP indexing configuration for code analysis
- Vector store configuration for semantic search over `.archon.md` files
- Code graph configuration for structural relationship traversal via GraphQL

These three new spec fields, along with corresponding status fields, allow the controller to declaratively manage the full two-layer knowledge system.

### Current CRD Baseline

**Spec fields:** `displayName`, `description`, `repositories[]`, `mcp`
**Status fields:** `phase`, `message`, `lastReconcileTime`, `mcp`

**Source:**
- `AphexControllerRuntime/api/v1alpha1/knowledgebase_types.go`
- `AphexControllerRuntime/crds/aphex.io_knowledgebases.yaml`

## Glossary

- **SCIP**: Source Code Intelligence Protocol — produces structured code analysis output
- **ARN**: Archon Resource Name — deterministic identifier linking vector results to code graph nodes
- **Vector Store**: Qdrant-based storage for semantic embeddings of `.archon.md` documentation
- **Code Graph**: PostgreSQL database with GraphQL API storing SCIP-derived code relationships
- **`.archon.md`**: Natural language documentation files generated from SCIP output, containing ARN references

## Requirements

### Requirement 1: SCIP Indexing Spec Field

**User Story:** As a platform operator, I want to configure SCIP indexing per KnowledgeBase, so that code analysis runs for the right languages.

#### Acceptance Criteria

1. THE KnowledgeBase CRD spec SHALL include a `scipIndexing` field as an optional object
2. THE `scipIndexing` field SHALL contain an `enabled` boolean (default: false)
3. THE `scipIndexing` field SHALL contain a `languages` list of strings
4. THE `languages` field SHALL validate against supported values: `go`, `typescript`, `python`, `java`
5. WHEN `scipIndexing.enabled` is false or omitted THEN the controller SHALL skip SCIP-related reconciliation

### Requirement 2: Vector Store Spec Field

**User Story:** As a platform operator, I want to configure the vector store source and embedding model per KnowledgeBase, so that semantic search uses the right data and model.

#### Acceptance Criteria

1. THE KnowledgeBase CRD spec SHALL include a `vectorStore` field as an optional object
2. THE `vectorStore` field SHALL contain a `source` enum with values: `kiro-docs`, `archon-docs` (default: `archon-docs`)
3. THE `vectorStore` field SHALL contain an `embeddingModel` string (default: `BAAI/bge-base-en-v1.5`)
4. THE `vectorStore` field SHALL contain an optional `collectionName` string
5. WHEN `vectorStore` is omitted THEN the controller SHALL not provision vector store resources

### Requirement 3: Code Graph Spec Field

**User Story:** As a platform operator, I want to configure the code graph endpoint per KnowledgeBase, so that structural relationship queries route to the correct service.

#### Acceptance Criteria

1. THE KnowledgeBase CRD spec SHALL include a `codeGraph` field as an optional object
2. THE `codeGraph` field SHALL contain an `enabled` boolean (default: false)
3. THE `codeGraph` field SHALL contain a `graphqlEndpoint` string
4. WHEN `codeGraph.enabled` is true THEN `graphqlEndpoint` SHALL be required
5. WHEN `codeGraph.enabled` is false or omitted THEN the controller SHALL skip code graph reconciliation

### Requirement 4: Knowledge Readiness Status Fields

**User Story:** As a platform operator, I want to observe the readiness of vector store and code graph components, so that I can verify the knowledge system is operational.

#### Acceptance Criteria

1. THE KnowledgeBase status SHALL include a `vectorStoreReady` boolean field
2. THE KnowledgeBase status SHALL include a `codeGraphReady` boolean field
3. THE KnowledgeBase status SHALL include a `lastSyncTime` timestamp field
4. THE controller SHALL set `vectorStoreReady` to true when the vector store is accessible and populated
5. THE controller SHALL set `codeGraphReady` to true when the code graph endpoint responds to health checks
6. THE controller SHALL update `lastSyncTime` after each successful sync cycle

### Requirement 5: Controller Reconciliation for New Fields

**User Story:** As a platform operator, I want the controller to reconcile the new SCIP knowledge fields, so that changes to the KnowledgeBase resource are reflected in the running system.

#### Acceptance Criteria

1. THE controller SHALL reconcile vector store configuration when the `vectorStore` field changes
2. THE controller SHALL reconcile code graph configuration when the `codeGraph` field changes
3. THE controller SHALL store vector store and code graph configuration in a ConfigMap alongside existing repository config
4. THE controller SHALL update status fields after each reconciliation cycle
5. WHEN reconciliation fails THEN the controller SHALL set the appropriate readiness field to false and update the status message
