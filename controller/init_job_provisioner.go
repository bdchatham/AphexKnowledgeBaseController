package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

func initJobLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "init-job",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "initialization",
	}
}

func buildInitJob(kb *platformv1alpha1.KnowledgeBase) *batchv1.Job {
	labels := initJobLabels(kb)
	secretName := externalSecretName(kb)
	pgHost := postgresServiceName(kb)
	qdrantHost := qdrantServiceName(kb)

	postgresCommand := `echo "Waiting for PostgreSQL to be ready..."
until pg_isready; do
  echo "PostgreSQL is not ready yet..."
  sleep 2
done
echo "PostgreSQL is ready. Creating schema..."
psql -c "
CREATE TABLE IF NOT EXISTS document_state (
    repo_file_path VARCHAR(512) PRIMARY KEY,
    sha VARCHAR(64) NOT NULL,
    last_modified TIMESTAMP NOT NULL,
    last_checked TIMESTAMP NOT NULL,
    content_hash VARCHAR(64)
);
CREATE INDEX IF NOT EXISTS idx_last_checked ON document_state(last_checked);
"
echo "PostgreSQL schema created successfully."`

	qdrantCommand := fmt.Sprintf(`echo "Waiting for Qdrant to be ready..."
until curl -f http://%s:%d/; do
  echo "Qdrant is not ready yet..."
  sleep 2
done
echo "Qdrant is ready. Creating collection..."
curl -X PUT http://%s:%d/collections/%s \
  -H "Content-Type: application/json" \
  -d '{
    "vectors": {
      "size": %d,
      "distance": "%s"
    }
  }'
echo "Qdrant collection created successfully."`,
		qdrantHost, QdrantHTTPPort,
		qdrantHost, QdrantHTTPPort, CollectionName,
		QdrantVectorSize, QdrantDistance)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      initJobName(kb),
			Namespace: infraNamespace(kb),
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:  "init-postgres",
							Image: InitPostgresImage,
							Env: []corev1.EnvVar{
								{
									Name:  "PGHOST",
									Value: pgHost,
								},
								{
									Name:  "PGPORT",
									Value: fmt.Sprintf("%d", PostgresPort),
								},
								{
									Name:  "PGDATABASE",
									Value: PostgresDBName,
								},
								{
									Name: "PGUSER",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "postgres_user",
										},
									},
								},
								{
									Name: "PGPASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "postgres_password",
										},
									},
								},
							},
							Command: []string{"/bin/sh", "-c", postgresCommand},
						},
						{
							Name:    "init-qdrant",
							Image:   InitCurlImage,
							Command: []string{"/bin/sh", "-c", qdrantCommand},
						},
					},
				},
			},
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcileInitJob(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)
	jobName := initJobName(kb)

	existing := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: infraNamespace(kb)}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createInitJob(ctx, kb)
		}
		return fmt.Errorf("failed to get init job: %w", err)
	}

	if existing.Status.Succeeded >= 1 {
		logger.V(1).Info("Init job already completed, skipping", "name", jobName)
		return nil
	}

	if existing.Status.Failed > 0 {
		logger.Info("Init job has failures, deleting for recreation", "name", jobName)
		propagation := metav1.DeletePropagationBackground
		if err := r.Delete(ctx, existing, &client.DeleteOptions{
			PropagationPolicy: &propagation,
		}); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete failed init job: %w", err)
		}
		return r.createInitJob(ctx, kb)
	}

	logger.V(1).Info("Init job still running", "name", jobName)
	return nil
}

func (r *KnowledgeBaseReconciler) createInitJob(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	job := buildInitJob(kb)
	if err := r.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to create init job: %w", err)
	}

	logger.Info("Created init job", "name", job.Name)
	return nil
}

func (r *KnowledgeBaseReconciler) cleanupInitJob(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	job := &batchv1.Job{}
	if err := r.Get(ctx, client.ObjectKey{Name: initJobName(kb), Namespace: infraNamespace(kb)}, job); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get init job for cleanup: %w", err)
	}

	propagation := metav1.DeletePropagationBackground
	if err := r.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: &propagation,
	}); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete init job: %w", err)
	}

	return nil
}
