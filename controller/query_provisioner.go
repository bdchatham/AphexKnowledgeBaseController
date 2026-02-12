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
			Namespace: infraNamespace(kb),
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
		envName      string
		configMapKey string
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
			Namespace: infraNamespace(kb),
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
	deploy := buildQueryDeployment(kb)
	svc := buildQueryService(kb)
	return r.reconcileWorkloadAndService(ctx, kb, deploy, svc, "query")
}

func (r *KnowledgeBaseReconciler) cleanupQuery(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploy := &appsv1.Deployment{}
	deploy.Name = queryDeploymentName(kb)
	deploy.Namespace = infraNamespace(kb)
	svc := &corev1.Service{}
	svc.Name = queryServiceName(kb)
	svc.Namespace = infraNamespace(kb)
	return r.cleanupWorkloadAndService(ctx, deploy, svc)
}
