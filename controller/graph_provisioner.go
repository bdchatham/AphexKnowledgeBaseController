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

func graphLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "graph",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "graph",
	}
}

func buildGraphDeployment(kb *platformv1alpha1.KnowledgeBase) *appsv1.Deployment {
	labels := graphLabels(kb)
	replicas := int32(GraphReplicas)
	configMapName := appConfigMapName(kb)
	secretName := externalSecretName(kb)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      graphDeploymentName(kb),
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
							Name:  "graph",
							Image: GraphImage,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(GraphPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: graphEnvVars(configMapName, secretName),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(GraphMemoryRequest),
									corev1.ResourceCPU:    mustParseQuantity(GraphCPURequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(GraphMemoryLimit),
									corev1.ResourceCPU:    mustParseQuantity(GraphCPULimit),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(int32(GraphPort)),
									},
								},
								FailureThreshold: 3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt32(int32(GraphPort)),
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

func graphEnvVars(configMapName, secretName string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "POSTGRES_HOST",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "postgres_host",
				},
			},
		},
		{
			Name: "POSTGRES_PORT",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "postgres_port",
				},
			},
		},
		{
			Name: "POSTGRES_DB",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "postgres_db",
				},
			},
		},
		{
			Name: "POSTGRES_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  "postgres_user",
				},
			},
		},
		{
			Name: "POSTGRES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  "postgres_password",
				},
			},
		},
	}
}

func buildGraphService(kb *platformv1alpha1.KnowledgeBase) *corev1.Service {
	labels := graphLabels(kb)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      graphServiceName(kb),
			Namespace: kb.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(GraphPort),
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcileGraph(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploy := buildGraphDeployment(kb)
	svc := buildGraphService(kb)
	return r.reconcileWorkloadAndService(ctx, kb, deploy, svc, "graph")
}

func (r *KnowledgeBaseReconciler) cleanupGraph(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploy := &appsv1.Deployment{}
	deploy.Name = graphDeploymentName(kb)
	deploy.Namespace = kb.Namespace
	svc := &corev1.Service{}
	svc.Name = graphServiceName(kb)
	svc.Namespace = kb.Namespace
	return r.cleanupWorkloadAndService(ctx, deploy, svc)
}
