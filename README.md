# AphexKnowledgeBaseController

Kubernetes controller for managing KnowledgeBase custom resources in the Aphex platform.

## Overview

The KnowledgeBaseController reconciles `KnowledgeBase` CRDs, managing:
- Repository configuration ConfigMaps
- MCP server deployments and services

## Dependencies

This controller depends on `AphexControllerRuntime` for shared types and utilities.

## Building

```bash
go mod tidy
go build ./...
```

## Running Locally

```bash
go run . --disable-leader-elect --development
```

## Docker Build

Build from the workspace root:

```bash
docker build -f AphexKnowledgeBaseController/Dockerfile -t knowledgebase-controller:latest .
```

## Deployment

Apply the manifests:

```bash
kubectl apply -f manifests/rbac.yaml
kubectl apply -f manifests/deployment.yaml
```
