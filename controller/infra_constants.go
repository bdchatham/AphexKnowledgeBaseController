package controller

const (
	// Qdrant vector database
	QdrantImage         = "qdrant/qdrant:v1.16.3"
	QdrantHTTPPort      = 6333
	QdrantGRPCPort      = 6334
	QdrantStorageSize   = "10Gi"
	QdrantMemoryRequest = "512Mi"
	QdrantMemoryLimit   = "1Gi"
	QdrantCPURequest    = "250m"

	// PostgreSQL database
	PostgresImage         = "postgres:15-alpine"
	PostgresPort          = 5432
	PostgresStorageSize   = "5Gi"
	PostgresMemoryRequest = "256Mi"
	PostgresMemoryLimit   = "512Mi"
	PostgresCPURequest    = "100m"
	PostgresDBName        = "archon"
	PostgresDBUser        = "archon"

	// Embedding service
	EmbeddingImage         = "ghcr.io/bdchatham/archon-knowledge-base:embedding-latest"
	EmbeddingPort          = 8000
	EmbeddingMemoryRequest = "2Gi"
	EmbeddingMemoryLimit   = "4Gi"
	EmbeddingCPURequest    = "500m"
	EmbeddingCPULimit      = "2000m"
	EmbeddingGPULimit      = "1"
	EmbeddingModel         = "BAAI/bge-base-en-v1.5"

	// Query service
	QueryImage         = "ghcr.io/bdchatham/archon-kb-query:latest"
	QueryPort          = 8080
	QueryReplicas      = 2
	QueryMemoryRequest = "256Mi"
	QueryMemoryLimit   = "512Mi"
	QueryCPURequest    = "100m"
	QueryCPULimit      = "500m"

	// Graph service
	GraphImage         = "ghcr.io/bdchatham/archon-knowledge-base:graph-latest"
	GraphPort          = 8081
	GraphReplicas      = 1
	GraphMemoryRequest = "256Mi"
	GraphMemoryLimit   = "512Mi"
	GraphCPURequest    = "100m"
	GraphCPULimit      = "500m"

	// Init job containers
	InitPostgresImage = "postgres:15-alpine"
	InitCurlImage     = "curlimages/curl:8.5.0"
	QdrantVectorSize  = 768
	QdrantDistance    = "Cosine"
	CollectionName    = "archon-docs"

	// Retrieval settings
	RetrievalK = "5"

	// HTTPRoute gateway references
	PlatformGatewayName        = "platform-gateway"
	PlatformGatewayNamespace   = "kong-system"
	PlatformGatewaySectionName = "https"
	PlatformAPISectionName     = "api"
	PlatformAPIDomain          = "aphex.arbiter-dev.com"

	// ExternalSecret configuration
	ExternalSecretRefreshInterval = "1h"
	OrgSecretsRemoteKey           = "org-secrets"
)
