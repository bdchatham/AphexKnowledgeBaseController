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

func embeddingLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "embedding",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "embedding",
	}
}

func buildEmbeddingDeployment(kb *platformv1alpha1.KnowledgeBase) *appsv1.Deployment {
	labels := embeddingLabels(kb)
	replicas := int32(1)
	configMapName := appConfigMapName(kb)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      embeddingDeploymentName(kb),
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
							Name:  "embedding",
							Image: EmbeddingImage,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(EmbeddingPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "EMBEDDING_MODEL",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "embedding_model",
										},
									},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(EmbeddingMemoryRequest),
									corev1.ResourceCPU:    mustParseQuantity(EmbeddingCPURequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(EmbeddingMemoryLimit),
									corev1.ResourceCPU:    mustParseQuantity(EmbeddingCPULimit),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(int32(EmbeddingPort)),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       30,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt32(int32(EmbeddingPort)),
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
						},
					},
				},
			},
		},
	}
}

func buildEmbeddingService(kb *platformv1alpha1.KnowledgeBase) *corev1.Service {
	labels := embeddingLabels(kb)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      embeddingServiceName(kb),
			Namespace: kb.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(EmbeddingPort),
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcileEmbedding(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	logger := log.FromContext(ctx)

	deploy := buildEmbeddingDeployment(kb)
	if err := controllerutil.SetControllerReference(kb, deploy, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on embedding deployment: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deploy.Name, Namespace: deploy.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, deploy); err != nil {
				return fmt.Errorf("failed to create embedding deployment: %w", err)
			}
			logger.Info("Created Embedding Deployment", "name", deploy.Name)
		} else {
			return fmt.Errorf("failed to get embedding deployment: %w", err)
		}
	} else {
		deploy.ResourceVersion = existing.ResourceVersion
		if err := r.Update(ctx, deploy); err != nil {
			return fmt.Errorf("failed to update embedding deployment: %w", err)
		}
	}

	svc := buildEmbeddingService(kb)
	if err := controllerutil.SetControllerReference(kb, svc, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on embedding service: %w", err)
	}

	existingSvc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, existingSvc)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, svc); err != nil {
				return fmt.Errorf("failed to create embedding service: %w", err)
			}
			logger.Info("Created Embedding Service", "name", svc.Name)
		} else {
			return fmt.Errorf("failed to get embedding service: %w", err)
		}
	} else {
		svc.ResourceVersion = existingSvc.ResourceVersion
		svc.Spec.ClusterIP = existingSvc.Spec.ClusterIP
		if err := r.Update(ctx, svc); err != nil {
			return fmt.Errorf("failed to update embedding service: %w", err)
		}
	}

	return nil
}

func (r *KnowledgeBaseReconciler) cleanupEmbedding(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Name: embeddingDeploymentName(kb), Namespace: kb.Namespace}, deploy); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get embedding deployment for cleanup: %w", err)
	}
	if err := r.Delete(ctx, deploy); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete embedding deployment: %w", err)
	}

	svc := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Name: embeddingServiceName(kb), Namespace: kb.Namespace}, svc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get embedding service for cleanup: %w", err)
	}
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete embedding service: %w", err)
	}

	return nil
}
