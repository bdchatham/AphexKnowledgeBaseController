package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

func queryLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "query",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "query",
	}
}

func buildQueryDeployment(kb *platformv1alpha1.KnowledgeBase) *appsv1.Deployment {
	labels := queryLabels(kb)
	replicas := int32(QueryReplicas)
	configMapName := appConfigMapName(kb)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      queryDeploymentName(kb),
			Namespace: kb.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "query",
							Image: QueryImage,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(QueryPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: queryEnvVars(configMapName),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(QueryMemoryRequest),
									corev1.ResourceCPU:    mustParseQuantity(QueryCPURequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(QueryMemoryLimit),
									corev1.ResourceCPU:    mustParseQuantity(QueryCPULimit),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(int32(QueryPort)),
									},
								},
								FailureThreshold: 3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt32(int32(QueryPort)),
									},
								},
								FailureThreshold: 3,
							},
						},
					},
				},
			},
		},
	}
}

func queryEnvVars(configMapName string) []corev1.EnvVar {
	keys := []struct {
		envName       string
		configMapKey  string
	}{
		{"EMBEDDING_SERVICE_URL", "embedding_service_url"},
		{"EMBEDDING_MODEL", "embedding_model"},
		{"VECTOR_DB_URL", "vector_db_url"},
		{"COLLECTION_NAME", "collection_name"},
		{"RETRIEVAL_K", "retrieval_k"},
	}

	envVars := make([]corev1.EnvVar, len(keys))
	for i, k := range keys {
		envVars[i] = corev1.EnvVar{
			Name: k.envName,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
					Key: k.configMapKey,
				},
			},
		}
	}
	return envVars
}

func buildQueryService(kb *platformv1alpha1.KnowledgeBase) *corev1.Service {
	labels := queryLabels(kb)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      queryServiceName(kb),
			Namespace: kb.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(QueryPort),
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcileQuery(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	deploy := buildQueryDeployment(kb)
	if err := controllerutil.SetControllerReference(kb, deploy, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on query deployment: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deploy.Name, Namespace: deploy.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, deploy); err != nil {
				return fmt.Errorf("failed to create query deployment: %w", err)
			}
			logger.Info("Created Query Deployment", "name", deploy.Name)
		} else {
			return fmt.Errorf("failed to get query deployment: %w", err)
		}
	} else {
		deploy.ResourceVersion = existing.ResourceVersion
		if err := r.Update(ctx, deploy); err != nil {
			return fmt.Errorf("failed to update query deployment: %w", err)
		}
	}

	svc := buildQueryService(kb)
	if err := controllerutil.SetControllerReference(kb, svc, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on query service: %w", err)
	}

	existingSvc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, existingSvc)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, svc); err != nil {
				return fmt.Errorf("failed to create query service: %w", err)
			}
			logger.Info("Created Query Service", "name", svc.Name)
		} else {
			return fmt.Errorf("failed to get query service: %w", err)
		}
	} else {
		svc.ResourceVersion = existingSvc.ResourceVersion
		svc.Spec.ClusterIP = existingSvc.Spec.ClusterIP
		if err := r.Update(ctx, svc); err != nil {
			return fmt.Errorf("failed to update query service: %w", err)
		}
	}

	return nil
}

func (r *KnowledgeBaseReconciler) cleanupQuery(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Name: queryDeploymentName(kb), Namespace: kb.Namespace}, deploy); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get query deployment for cleanup: %w", err)
	}
	if err := r.Delete(ctx, deploy); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete query deployment: %w", err)
	}

	svc := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Name: queryServiceName(kb), Namespace: kb.Namespace}, svc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get query service for cleanup: %w", err)
	}
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete query service: %w", err)
	}

	return nil
}
