package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

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
			Namespace: infraNamespace(kb),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RuntimeClassName: stringPtr("nvidia"),
					Containers: []corev1.Container{
						{
							Name:            "embedding",
							Image:           EmbeddingImage,
							ImagePullPolicy: corev1.PullAlways,
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
									corev1.ResourceMemory:                    mustParseQuantity(EmbeddingMemoryLimit),
									corev1.ResourceCPU:                       mustParseQuantity(EmbeddingCPULimit),
									corev1.ResourceName("nvidia.com/gpu"):    mustParseQuantity(EmbeddingGPULimit),
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
								TimeoutSeconds:      10,
								FailureThreshold:    6,
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
			Namespace: infraNamespace(kb),
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
	deploy := buildEmbeddingDeployment(kb)
	svc := buildEmbeddingService(kb)
	return r.reconcileWorkloadAndService(ctx, deploy, svc, "embedding")
}

func (r *KnowledgeBaseReconciler) cleanupEmbedding(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploy := &appsv1.Deployment{}
	deploy.Name = embeddingDeploymentName(kb)
	deploy.Namespace = infraNamespace(kb)
	svc := &corev1.Service{}
	svc.Name = embeddingServiceName(kb)
	svc.Namespace = infraNamespace(kb)
	return r.cleanupWorkloadAndService(ctx, deploy, svc)
}
