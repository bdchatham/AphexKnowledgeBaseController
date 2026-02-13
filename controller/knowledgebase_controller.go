package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/config"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
	"github.com/bdchatham/AphexControllerRuntime/pkg/helpers"
	"github.com/bdchatham/AphexControllerRuntime/pkg/metrics"
)

const healthCheckTimeout = 5 * time.Second

// HTTPClient abstracts HTTP operations for testability.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// KnowledgeBaseReconciler reconciles a KnowledgeBase object
type KnowledgeBaseReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Log             logr.Logger
	Config          *config.Config
	HTTPClient      HTTPClient
	statusHelper    *helpers.StatusHelper
	finalizerHelper *helpers.FinalizerHelper
}

// +kubebuilder:rbac:groups=aphex.io,resources=knowledgebases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aphex.io,resources=knowledgebases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aphex.io,resources=knowledgebases/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile manages KnowledgeBase resources
func (r *KnowledgeBaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) { //nolint:gocyclo
	logger := log.FromContext(ctx)

	metricsCollector := metrics.GetCollector()
	timer := metricsCollector.NewReconcileTimer(constants.ControllerNameKnowledgeBase)

	if err := r.ensureHelpers(logger); err != nil {
		logger.Error(err, "Failed to initialize helpers")
		timer.ObserveError(metrics.ClassifyError(err))
		return ctrl.Result{}, err
	}

	timeout := constants.DefaultProvisioningTimeout
	if r.Config != nil {
		timeout = r.Config.ProvisioningTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	kb := &platformv1alpha1.KnowledgeBase{}
	err := r.Get(ctx, req.NamespacedName, kb)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get KnowledgeBase")
		timer.ObserveError(metrics.ClassifyError(err))
		return ctrl.Result{}, err
	}

	if r.finalizerHelper.IsBeingDeleted(kb) {
		result, err := r.handleDeletionWithHelper(ctx, logger, kb)
		if err != nil {
			timer.ObserveError(metrics.ClassifyError(err))
		} else {
			timer.ObserveSuccess()
			metrics.GetHealthState().RecordSuccess(constants.ControllerNameKnowledgeBase)
		}
		return result, err
	}

	if !r.finalizerHelper.HasFinalizer(kb) {
		if err := r.validateSpec(kb); err != nil {
			logger.Error(err, "Validation failed, not adding finalizer")
			timer.ObserveError("validation")
			if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
				"phase":   constants.PhaseFailed,
				"message": fmt.Sprintf("Validation failed: %s", err.Error()),
			}); patchErr != nil {
				logger.Error(patchErr, "Failed to update status after validation failure")
			}
			return ctrl.Result{}, nil
		}
		if err := r.finalizerHelper.EnsureFinalizer(ctx, kb); err != nil {
			timer.ObserveError(metrics.ClassifyError(err))
			return ctrl.Result{}, err
		}
		timer.ObserveSuccess()
		return ctrl.Result{Requeue: true}, nil
	}

	if kb.Status.Phase == "" {
		if err := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhasePending,
			"message":           "Starting KnowledgeBase provisioning",
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); err != nil {
			logger.Error(err, "Failed to update KnowledgeBase status to Pending")
			timer.ObserveError(metrics.ClassifyError(err))
			return ctrl.Result{}, err
		}
		logger.Info("Set KnowledgeBase phase to Pending", "name", kb.Name, "namespace", kb.Namespace)
		timer.ObserveSuccess()
		return ctrl.Result{Requeue: true}, nil
	}

	if kb.Status.Phase == constants.PhasePending {
		if err := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseProvisioning,
			"message":           "Provisioning KnowledgeBase resources",
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); err != nil {
			timer.ObserveError(metrics.ClassifyError(err))
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileNamespace(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile infrastructure namespace")
		timer.ObserveError(metrics.ClassifyError(err))
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Namespace provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after namespace failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "namespace", "success")

	select {
	case <-ctx.Done():
		timer.ObserveError("canceled")
		return ctrl.Result{}, fmt.Errorf("context canceled before provisioning: %w", ctx.Err())
	default:
	}

	if err := r.reconcileExternalSecret(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile external secret")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "external_secret", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("External secret provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after external secret failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "external_secret", "success")

	if err := r.reconcileQdrant(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile Qdrant")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "qdrant", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Qdrant provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after Qdrant failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "qdrant", "success")

	if err := r.reconcilePostgres(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile Postgres")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "postgres", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Postgres provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after Postgres failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "postgres", "success")

	if err := r.reconcileInitJob(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile init job")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "init_job", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Init job provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after init job failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "init_job", "success")

	if err := r.reconcileAppConfig(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile app config")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "app_config", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("App config provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after app config failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "app_config", "success")

	if err := r.reconcileEmbedding(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile embedding")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "embedding", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Embedding provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after embedding failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "embedding", "success")

	if err := r.reconcileQuery(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile query")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "query", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Query provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after query failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "query", "success")

	if err := r.reconcileGraph(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile graph")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "graph", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Graph provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after graph failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "graph", "success")

	if err := r.reconcileHTTPRoute(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile HTTP route")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "httproute", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("HTTP route provisioning failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after HTTP route failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "httproute", "success")

	if err := r.reconcileRepositoryConfig(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile repository configuration")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "repository_config", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Repository configuration failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after repository config failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "repository_config", "success")

	if err := r.reconcileSourceConfig(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile source configuration")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "source_config", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Source configuration failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after source config failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "source_config", "success")

	if err := r.reconcileRepoMapping(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile repo mapping")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "repo_mapping", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Repo mapping reconciliation failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after repo mapping failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "repo_mapping", "success")

	if err := r.reconcileTriggerTemplate(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile TriggerTemplate")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "trigger_template", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("TriggerTemplate reconciliation failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after TriggerTemplate failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "trigger_template", "success")

	if err := r.reconcileTrigger(ctx, kb); err != nil {
		logger.Error(err, "Failed to reconcile Trigger")
		timer.ObserveError(metrics.ClassifyError(err))
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "trigger", "error")
		if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
			"phase":             constants.PhaseFailed,
			"message":           fmt.Sprintf("Trigger reconciliation failed: %v", err),
			"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		}); patchErr != nil {
			logger.Error(patchErr, "Failed to update status after Trigger failure")
		}
		return ctrl.Result{}, err
	}
	metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "trigger", "success")

	r.checkVectorStoreHealth(ctx, kb)
	r.checkCodeGraphHealth(ctx, kb)

	select {
	case <-ctx.Done():
		timer.ObserveError("canceled")
		return ctrl.Result{}, fmt.Errorf("context canceled during provisioning: %w", ctx.Err())
	default:
	}

	if kb.Spec.MCP != nil {
		if err := r.reconcileMCPServer(ctx, kb); err != nil {
			logger.Error(err, "Failed to reconcile MCP server")
			timer.ObserveError(metrics.ClassifyError(err))
			metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "mcp_server", "error")
			if patchErr := r.statusHelper.PatchStatus(ctx, kb, map[string]interface{}{
				"phase":             constants.PhaseFailed,
				"message":           fmt.Sprintf("MCP server reconciliation failed: %v", err),
				"lastReconcileTime": metav1.Now().Format(time.RFC3339),
			}); patchErr != nil {
				logger.Error(patchErr, "Failed to update status after MCP server failure")
			}
			return ctrl.Result{}, err
		}
		metricsCollector.RecordProvisioningStep(constants.ControllerNameKnowledgeBase, "mcp_server", "success")
	} else {
		if err := r.cleanupMCPServer(ctx, kb); err != nil {
			logger.Error(err, "Failed to cleanup MCP server")
			timer.ObserveError(metrics.ClassifyError(err))
			return ctrl.Result{}, err
		}
	}

	statusPatch := map[string]interface{}{
		"phase":             constants.PhaseReady,
		"message":           fmt.Sprintf("Tracking %d sources", len(kb.Spec.Sources)),
		"lastReconcileTime": metav1.Now().Format(time.RFC3339),
		"vectorStoreReady":  kb.Status.VectorStoreReady,
		"codeGraphReady":    kb.Status.CodeGraphReady,
	}

	if kb.Spec.MCP != nil {
		statusPatch["mcp"] = map[string]interface{}{
			"deployed":      kb.Status.MCP.Deployed,
			"serviceName":   kb.Status.MCP.ServiceName,
			"serviceURL":    kb.Status.MCP.ServiceURL,
			"readyReplicas": kb.Status.MCP.ReadyReplicas,
		}
	}

	if err := r.statusHelper.PatchStatus(ctx, kb, statusPatch); err != nil {
		logger.Error(err, "Failed to update KnowledgeBase status to Ready")
		timer.ObserveError(metrics.ClassifyError(err))
		return ctrl.Result{}, err
	}

	if kb.Status.LastSyncTime == nil {
		if err := r.createInitialSyncTaskRun(ctx, kb); err != nil {
			logger.Error(err, "Failed to create initial sync TaskRun")
		}
	}

	timer.ObserveSuccess()
	metrics.GetHealthState().RecordSuccess(constants.ControllerNameKnowledgeBase)
	logger.Info("KnowledgeBase reconciled successfully",
		"name", kb.Name,
		"namespace", kb.Namespace,
		"sources", len(kb.Spec.Sources),
	)

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *KnowledgeBaseReconciler) ensureHelpers(logger logr.Logger) error { //nolint:unparam
	if r.statusHelper == nil {
		retryCount := constants.DefaultStatusRetryCount
		if r.Config != nil {
			retryCount = r.Config.StatusRetryCount
		}
		r.statusHelper = helpers.NewStatusHelper(r.Client, logger, retryCount)
	}

	if r.finalizerHelper == nil {
		r.finalizerHelper = helpers.NewFinalizerHelper(r.Client, logger, constants.KnowledgeBaseFinalizer)
	}

	return nil
}

func (r *KnowledgeBaseReconciler) validateSpec(kb *platformv1alpha1.KnowledgeBase) error {
	if err := kb.ValidateOrganization(); err != nil {
		return err
	}

	if len(kb.Spec.Sources) == 0 {
		return fmt.Errorf("sources array cannot be empty")
	}

	for i, source := range kb.Spec.Sources {
		if source.URL == "" {
			return fmt.Errorf("source[%d]: URL cannot be empty", i)
		}

		if !strings.HasPrefix(source.URL, "http://") && !strings.HasPrefix(source.URL, "https://") {
			return fmt.Errorf("source[%d]: URL must start with http:// or https://", i)
		}

		if source.Branch != "" && !isValidBranchName(source.Branch) {
			return fmt.Errorf("source[%d]: invalid branch name '%s'", i, source.Branch)
		}

		sourceType := defaultSourceType(source.SourceType)
		if sourceType != sourceTypeDocs && sourceType != sourceTypeCode {
			return fmt.Errorf("source[%d]: sourceType must be 'docs' or 'code', got '%s'", i, source.SourceType)
		}
	}

	return nil
}

func (r *KnowledgeBaseReconciler) handleDeletionWithHelper(ctx context.Context, logger logr.Logger, kb *platformv1alpha1.KnowledgeBase) (ctrl.Result, error) { //nolint:unparam
	if !r.finalizerHelper.NeedsCleanup(kb) {
		return ctrl.Result{}, nil
	}

	cleanupSteps := []helpers.CleanupStep{
		helpers.NewCleanupStep("HTTPRoute", func(ctx context.Context) error {
			return r.cleanupHTTPRoute(ctx, kb)
		}),
		helpers.NewCleanupStep("Query service", func(ctx context.Context) error {
			return r.cleanupQuery(ctx, kb)
		}),
		helpers.NewCleanupStep("Graph service", func(ctx context.Context) error {
			return r.cleanupGraph(ctx, kb)
		}),
		helpers.NewCleanupStep("Embedding service", func(ctx context.Context) error {
			return r.cleanupEmbedding(ctx, kb)
		}),
		helpers.NewCleanupStep("Init job", func(ctx context.Context) error {
			return r.cleanupInitJob(ctx, kb)
		}),
		helpers.NewCleanupStep("Postgres", func(ctx context.Context) error {
			return r.cleanupPostgres(ctx, kb)
		}),
		helpers.NewCleanupStep("Qdrant", func(ctx context.Context) error {
			return r.cleanupQdrant(ctx, kb)
		}),
		helpers.NewCleanupStep("External secret", func(ctx context.Context) error {
			return r.cleanupExternalSecret(ctx, kb)
		}),
		helpers.NewCleanupStep("App config", func(ctx context.Context) error {
			return r.cleanupAppConfig(ctx, kb)
		}),
		helpers.NewCleanupStep("PVCs", func(ctx context.Context) error {
			return r.cleanupPVCs(ctx, kb)
		}),
		helpers.NewCleanupStep("Repository configuration", func(ctx context.Context) error {
			return r.cleanupRepositoryConfig(ctx, kb)
		}),
		helpers.NewCleanupStep("Source configuration", func(ctx context.Context) error {
			return r.cleanupSourceConfig(ctx, kb)
		}),
		helpers.NewCleanupStep("Repo mapping", func(ctx context.Context) error {
			return r.cleanupRepoMapping(ctx, kb)
		}),
		helpers.NewCleanupStep("Triggers", func(ctx context.Context) error {
			return r.cleanupTriggers(ctx, kb)
		}),
		helpers.NewCleanupStep("MCP server resources", func(ctx context.Context) error {
			return r.cleanupMCPServerResources(ctx, kb)
		}),
		helpers.NewCleanupStep("Infrastructure namespace", func(ctx context.Context) error {
			return r.cleanupNamespace(ctx, kb)
		}),
	}

	if err := r.finalizerHelper.HandleDeletionWithSteps(ctx, kb, cleanupSteps); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("KnowledgeBase deletion completed", "knowledgebase", kb.Name)
	return ctrl.Result{}, nil
}

func (r *KnowledgeBaseReconciler) cleanupMCPServerResources(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploymentName := fmt.Sprintf("mcp-server-%s", kb.Name)
	serviceName := fmt.Sprintf("mcp-server-%s", kb.Name)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: infraNamespace(kb)}, deployment); err == nil {
		if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete MCP server deployment: %w", err)
		}
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get MCP server deployment for cleanup: %w", err)
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: infraNamespace(kb)}, service); err == nil {
		if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete MCP server service: %w", err)
		}
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get MCP server service for cleanup: %w", err)
	}

	return nil
}

func (r *KnowledgeBaseReconciler) reconcileRepositoryConfig(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled during repository config reconciliation: %w", ctx.Err())
	default:
	}

	configMapName := fmt.Sprintf("%s-repos", kb.Name)
	repoData := r.buildRepositoryConfigData(kb)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: infraNamespace(kb),
			Labels: map[string]string{
				constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
				"knowledgebase":               kb.Name,
				"app.kubernetes.io/name":      "knowledgebase-config",
				"app.kubernetes.io/instance":  kb.Name,
				"app.kubernetes.io/part-of":   "archon",
				"app.kubernetes.io/component": "config",
			},
		},
		Data: repoData,
	}

	return r.reconcileConfigMap(ctx, configMap, "repository-config")
}

func (r *KnowledgeBaseReconciler) buildRepositoryConfigData(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	data := make(map[string]string)

	data["name"] = kb.Spec.Name
	if kb.Spec.Description != "" {
		data["description"] = kb.Spec.Description
	}

	data["sourceCount"] = fmt.Sprintf("%d", len(kb.Spec.Sources))

	for i, source := range kb.Spec.Sources {
		prefix := fmt.Sprintf("repo.%d.", i)
		data[prefix+"url"] = source.URL

		branch := source.Branch
		if branch == "" {
			branch = "mainline"
		}
		data[prefix+"branch"] = branch

		data[prefix+"sourceType"] = defaultSourceType(source.SourceType)

		if len(source.Paths) > 0 {
			data[prefix+"paths"] = strings.Join(source.Paths, ",")
		} else {
			data[prefix+"paths"] = defaultSourcePath
		}
	}

	return data
}

func (r *KnowledgeBaseReconciler) cleanupRepositoryConfig(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	configMapName := fmt.Sprintf("%s-repos", kb.Name)

	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: infraNamespace(kb)}, configMap); err == nil {
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete repository configuration ConfigMap: %w", err)
		}
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get repository configuration ConfigMap for cleanup: %w", err)
	}

	return nil
}

func (r *KnowledgeBaseReconciler) reconcileSourceConfig(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled during source config reconciliation: %w", ctx.Err())
	default:
	}

	configMapName := fmt.Sprintf("%s-source-config", kb.Name)
	sourceData := r.buildSourceConfigData(kb)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: infraNamespace(kb),
			Labels: map[string]string{
				constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
				"knowledgebase":               kb.Name,
				"app.kubernetes.io/name":      "knowledgebase-source-config",
				"app.kubernetes.io/instance":  kb.Name,
				"app.kubernetes.io/part-of":   "archon",
				"app.kubernetes.io/component": "config",
			},
		},
		Data: sourceData,
	}

	return r.reconcileConfigMap(ctx, configMap, "source-config")
}

func (r *KnowledgeBaseReconciler) buildSourceConfigData(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	data := make(map[string]string)

	for i, source := range kb.Spec.Sources {
		prefix := fmt.Sprintf("source.%d.", i)
		data[prefix+"url"] = source.URL
		data[prefix+"sourceType"] = defaultSourceType(source.SourceType)

		if len(source.Paths) > 0 {
			data[prefix+"paths"] = strings.Join(source.Paths, ",")
		}
	}

	if hasCodeSources(kb.Spec.Sources) {
		data["codeGraph.endpoint"] = fmt.Sprintf("http://code-graph.%s:5432", infraNamespace(kb))
	}

	return data
}

func (r *KnowledgeBaseReconciler) cleanupSourceConfig(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	configMapName := fmt.Sprintf("%s-source-config", kb.Name)

	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: infraNamespace(kb)}, configMap); err == nil {
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete source configuration ConfigMap: %w", err)
		}
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get source configuration ConfigMap for cleanup: %w", err)
	}

	return nil
}

func (r *KnowledgeBaseReconciler) reconcileMCPServer(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled during MCP server reconciliation: %w", ctx.Err())
	default:
	}

	image := kb.Spec.MCP.Image
	port := kb.Spec.MCP.Port

	queryServiceURL := kb.Spec.MCP.QueryServiceURL
	if queryServiceURL == "" {
		queryServiceURL = fmt.Sprintf("http://query.%s:8080", infraNamespace(kb))
	}

	replicas := kb.Spec.MCP.Replicas
	if replicas == 0 {
		replicas = 1
	}

	deploymentName := fmt.Sprintf("mcp-server-%s", kb.Name)
	serviceName := fmt.Sprintf("mcp-server-%s", kb.Name)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: infraNamespace(kb),
			Labels: map[string]string{
				constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
				"app":                         "mcp-server",
				"knowledgebase":               kb.Name,
				"app.kubernetes.io/name":      "mcp-server",
				"app.kubernetes.io/instance":  kb.Name,
				"app.kubernetes.io/part-of":   "archon",
				"app.kubernetes.io/component": "mcp",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":                         "mcp-server",
					"knowledgebase":               kb.Name,
					"app.kubernetes.io/name":      "mcp-server",
					"app.kubernetes.io/instance":  kb.Name,
					"app.kubernetes.io/part-of":   "archon",
					"app.kubernetes.io/component": "mcp",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                         "mcp-server",
						"knowledgebase":               kb.Name,
						"app.kubernetes.io/name":      "mcp-server",
						"app.kubernetes.io/instance":  kb.Name,
						"app.kubernetes.io/part-of":   "archon",
						"app.kubernetes.io/component": "mcp",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mcp-server",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "QUERY_SERVICE_URL",
									Value: queryServiceURL,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity("128Mi"),
									corev1.ResourceCPU:    mustParseQuantity("50m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity("256Mi"),
									corev1.ResourceCPU:    mustParseQuantity("200m"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
						},
					},
				},
			},
		},
	}

	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: infraNamespace(kb)}, existingDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, deployment); err != nil {
				return fmt.Errorf("failed to create deployment: %w", err)
			}
			logger.Info("Created MCP server deployment", "name", deploymentName, "namespace", infraNamespace(kb))
		} else {
			return fmt.Errorf("failed to get deployment: %w", err)
		}
	} else {
		deployment.ResourceVersion = existingDeployment.ResourceVersion
		if err := r.Update(ctx, deployment); err != nil {
			return fmt.Errorf("failed to update deployment: %w", err)
		}
		logger.V(1).Info("Updated MCP server deployment", "name", deploymentName, "namespace", infraNamespace(kb))
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled during MCP server service reconciliation: %w", ctx.Err())
	default:
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: infraNamespace(kb),
			Labels: map[string]string{
				constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
				"app":                         "mcp-server",
				"knowledgebase":               kb.Name,
				"app.kubernetes.io/name":      "mcp-server",
				"app.kubernetes.io/instance":  kb.Name,
				"app.kubernetes.io/part-of":   "archon",
				"app.kubernetes.io/component": "mcp",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app":           "mcp-server",
				"knowledgebase": kb.Name,
			},
		},
	}

	existingService := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: infraNamespace(kb)}, existingService)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, service); err != nil {
				return fmt.Errorf("failed to create service: %w", err)
			}
			logger.Info("Created MCP server service", "name", serviceName, "namespace", infraNamespace(kb))
		} else {
			return fmt.Errorf("failed to get service: %w", err)
		}
	} else {
		service.ResourceVersion = existingService.ResourceVersion
		service.Spec.ClusterIP = existingService.Spec.ClusterIP
		if err := r.Update(ctx, service); err != nil {
			return fmt.Errorf("failed to update service: %w", err)
		}
		logger.V(1).Info("Updated MCP server service", "name", serviceName, "namespace", infraNamespace(kb))
	}

	kb.Status.MCP.Deployed = true
	kb.Status.MCP.ServiceName = serviceName
	kb.Status.MCP.ServiceURL = fmt.Sprintf("http://%s.%s:%d", serviceName, infraNamespace(kb), port)
	if existingDeployment.Status.ReadyReplicas > 0 {
		kb.Status.MCP.ReadyReplicas = existingDeployment.Status.ReadyReplicas
	}

	return nil
}

func (r *KnowledgeBaseReconciler) httpClient() HTTPClient {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return &http.Client{Timeout: healthCheckTimeout}
}

func (r *KnowledgeBaseReconciler) checkVectorStoreHealth(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) {
	logger := log.FromContext(ctx)

	endpoint := fmt.Sprintf("http://qdrant.%s:6333/health", infraNamespace(kb))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		logger.V(1).Info("Failed to create vector store health request", "error", err)
		kb.Status.VectorStoreReady = false
		return
	}

	resp, err := r.httpClient().Do(req)
	if err != nil {
		logger.V(1).Info("Vector store health check failed", "endpoint", endpoint, "error", err)
		kb.Status.VectorStoreReady = false
		return
	}
	defer resp.Body.Close()

	kb.Status.VectorStoreReady = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !kb.Status.VectorStoreReady {
		logger.V(1).Info("Vector store health check returned non-2xx", "endpoint", endpoint, "status", resp.StatusCode)
	}
}

func (r *KnowledgeBaseReconciler) checkCodeGraphHealth(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) {
	if !hasCodeSources(kb.Spec.Sources) {
		kb.Status.CodeGraphReady = false
		return
	}

	logger := log.FromContext(ctx)

	endpoint := fmt.Sprintf("http://code-graph.%s:5432/health", infraNamespace(kb))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		logger.V(1).Info("Failed to create code graph health request", "error", err)
		kb.Status.CodeGraphReady = false
		return
	}

	resp, err := r.httpClient().Do(req)
	if err != nil {
		logger.V(1).Info("Code graph health check failed", "endpoint", endpoint, "error", err)
		kb.Status.CodeGraphReady = false
		return
	}
	defer resp.Body.Close()

	kb.Status.CodeGraphReady = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !kb.Status.CodeGraphReady {
		logger.V(1).Info("Code graph health check returned non-2xx", "endpoint", endpoint, "status", resp.StatusCode)
	}
}

func (r *KnowledgeBaseReconciler) reconcileRepoMapping(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	mappingValue := fmt.Sprintf("%s/%s", kb.Name, infraNamespace(kb))

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: repoMappingConfigMapName, Namespace: orgNamespace(kb)}, configMap)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get repo mapping ConfigMap: %w", err)
		}
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      repoMappingConfigMapName,
				Namespace: orgNamespace(kb),
				Labels: map[string]string{
					constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
					constants.LabelOrganization:   kb.Spec.Organization,
					"app.kubernetes.io/name":      "repo-mapping",
					"app.kubernetes.io/component": "event-routing",
				},
			},
			Data: make(map[string]string),
		}
	}

	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	removeEntriesForKnowledgeBase(configMap.Data, mappingValue)

	for _, source := range kb.Spec.Sources {
		configMap.Data[repoMappingKey(source.URL)] = mappingValue
	}

	if err != nil {
		if createErr := r.Create(ctx, configMap); createErr != nil {
			return fmt.Errorf("failed to create repo mapping ConfigMap: %w", createErr)
		}
		logger.Info("Created repo mapping ConfigMap", "namespace", orgNamespace(kb))
		return nil
	}

	if updateErr := r.Update(ctx, configMap); updateErr != nil {
		return fmt.Errorf("failed to update repo mapping ConfigMap: %w", updateErr)
	}
	logger.V(1).Info("Updated repo mapping ConfigMap", "namespace", orgNamespace(kb), "knowledgebase", kb.Name)
	return nil
}

func (r *KnowledgeBaseReconciler) cleanupRepoMapping(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	mappingValue := fmt.Sprintf("%s/%s", kb.Name, infraNamespace(kb))

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: repoMappingConfigMapName, Namespace: orgNamespace(kb)}, configMap)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get repo mapping ConfigMap for cleanup: %w", err)
	}

	if configMap.Data == nil {
		return nil
	}

	removeEntriesForKnowledgeBase(configMap.Data, mappingValue)

	if err := r.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update repo mapping ConfigMap during cleanup: %w", err)
	}
	return nil
}

func removeEntriesForKnowledgeBase(data map[string]string, mappingValue string) {
	for key, value := range data {
		if value == mappingValue {
			delete(data, key)
		}
	}
}

func repoMappingKey(repoURL string) string {
	trimmed := strings.TrimPrefix(repoURL, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimSuffix(trimmed, ".git")
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-2] + "_" + parts[len(parts)-1]
	}
	return strings.ReplaceAll(trimmed, "/", "_")
}

func (r *KnowledgeBaseReconciler) cleanupMCPServer(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	deploymentName := fmt.Sprintf("mcp-server-%s", kb.Name)
	serviceName := fmt.Sprintf("mcp-server-%s", kb.Name)

	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: infraNamespace(kb)}, deployment)
	if err == nil {
		if err := r.Delete(ctx, deployment); err != nil {
			return fmt.Errorf("failed to delete deployment: %w", err)
		}
		logger.Info("Deleted MCP server deployment", "name", deploymentName, "namespace", infraNamespace(kb))
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get deployment for cleanup: %w", err)
	}

	service := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: infraNamespace(kb)}, service)
	if err == nil {
		if err := r.Delete(ctx, service); err != nil {
			return fmt.Errorf("failed to delete service: %w", err)
		}
		logger.Info("Deleted MCP server service", "name", serviceName, "namespace", infraNamespace(kb))
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get service for cleanup: %w", err)
	}

	kb.Status.MCP.Deployed = false
	kb.Status.MCP.ServiceName = ""
	kb.Status.MCP.ServiceURL = ""
	kb.Status.MCP.ReadyReplicas = 0

	return nil
}

func (r *KnowledgeBaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerAndOptions(mgr, &controller.Options{})
}

func (r *KnowledgeBaseReconciler) SetupWithManagerAndOptions(mgr ctrl.Manager, opts *controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.KnowledgeBase{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&batchv1.Job{}).
		WithOptions(*opts).
		Complete(r)
}
