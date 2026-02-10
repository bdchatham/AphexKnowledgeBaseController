# Implementation Plan: SCIP Knowledge Integration

## Overview

Extends the KnowledgeBase CRD and controller to support SCIP-driven knowledge configuration (vector store, code graph, SCIP indexing).

**Parent Spec:** `ArchonAgent/.kiro/specs/agent-orchestration/` (Requirement 13)

## Tasks

- [ ] 1. Add SCIP knowledge types to AphexControllerRuntime
  - [ ] 1.1 Add SCIPIndexingConfig, VectorStoreConfig, CodeGraphConfig structs to knowledgebase_types.go
    - _Requirements: 1.1-1.4, 2.1-2.4, 3.1-3.4_
  - [ ] 1.2 Add new fields to KnowledgeBaseSpec (scipIndexing, vectorStore, codeGraph)
    - _Requirements: 1.1, 2.1, 3.1_
  - [ ] 1.3 Add vectorStoreReady, codeGraphReady, lastSyncTime to KnowledgeBaseStatus
    - _Requirements: 4.1-4.3_
  - [ ] 1.4 Regenerate deep copy and CRD YAML
  - [ ] 1.5 Add printer columns for vectorStoreReady and codeGraphReady

- [ ] 2. Update controller reconciliation
  - [ ] 2.1 Add validation for new spec fields in validateSpec
    - _Requirements: 1.5, 3.4_
  - [ ] 2.2 Implement reconcileKnowledgeConfig method
    - _Requirements: 5.1-5.3_
  - [ ] 2.3 Update status patch to include new readiness fields
    - _Requirements: 4.4-4.6, 5.4-5.5_
  - [ ] 2.4 Wire reconcileKnowledgeConfig into the reconciliation loop

- [ ] 3. Write tests
  - [ ] 3.1 Unit tests for new validation logic
  - [ ] 3.2 Unit tests for reconcileKnowledgeConfig
  - [ ] 3.3 Property tests for CRD field validation invariants
