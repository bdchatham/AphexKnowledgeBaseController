package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

func qdrantLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "qdrant",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "vector-database",
	}
}

func postgresLabels(kb *platformv1alpha1.KnowledgeBase) map[string]string {
	return map[string]string{
		constants.LabelManagedBy:      constants.ManagedByKnowledgeBaseController,
		"knowledgebase":               kb.Name,
		"app.kubernetes.io/name":      "postgres",
		"app.kubernetes.io/instance":  kb.Name,
		"app.kubernetes.io/part-of":   "archon",
		"app.kubernetes.io/component": "database",
	}
}

func buildQdrantStatefulSet(kb *platformv1alpha1.KnowledgeBase) *appsv1.StatefulSet {
	labels := qdrantLabels(kb)
	replicas := int32(1)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      qdrantStatefulSetName(kb),
			Namespace: infraNamespace(kb),
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: qdrantServiceName(kb),
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
							Name:  "qdrant",
							Image: QdrantImage,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(QdrantHTTPPort),
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "grpc",
									ContainerPort: int32(QdrantGRPCPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(QdrantMemoryRequest),
									corev1.ResourceCPU:    mustParseQuantity(QdrantCPURequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(QdrantMemoryLimit),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt32(int32(QdrantHTTPPort)),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt32(int32(QdrantHTTPPort)),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "qdrant-storage",
									MountPath: "/qdrant/storage",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "qdrant-storage",
						Labels: map[string]string{
							"knowledgebase": kb.Name,
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(QdrantStorageSize),
							},
						},
					},
				},
			},
		},
	}
}

func buildQdrantService(kb *platformv1alpha1.KnowledgeBase) *corev1.Service {
	labels := qdrantLabels(kb)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      qdrantServiceName(kb),
			Namespace: infraNamespace(kb),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(QdrantHTTPPort),
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "grpc",
					Port:       int32(QdrantGRPCPort),
					TargetPort: intstr.FromString("grpc"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcileQdrant(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	ss := buildQdrantStatefulSet(kb)
	svc := buildQdrantService(kb)
	return r.reconcileWorkloadAndService(ctx, kb, ss, svc, "qdrant")
}

func (r *KnowledgeBaseReconciler) cleanupQdrant(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	ss := &appsv1.StatefulSet{}
	ss.Name = qdrantStatefulSetName(kb)
	ss.Namespace = infraNamespace(kb)
	svc := &corev1.Service{}
	svc.Name = qdrantServiceName(kb)
	svc.Namespace = infraNamespace(kb)
	return r.cleanupWorkloadAndService(ctx, ss, svc)
}

func buildPostgresStatefulSet(kb *platformv1alpha1.KnowledgeBase) *appsv1.StatefulSet {
	labels := postgresLabels(kb)
	replicas := int32(1)
	secretName := externalSecretName(kb)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresStatefulSetName(kb),
			Namespace: infraNamespace(kb),
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: postgresServiceName(kb),
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
							Name:  "postgres",
							Image: PostgresImage,
							Ports: []corev1.ContainerPort{
								{
									Name:          "tcp",
									ContainerPort: int32(PostgresPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_DB",
									Value: PostgresDBName,
								},
								{
									Name: "POSTGRES_USER",
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
									Name: "POSTGRES_PASSWORD",
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
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(PostgresMemoryRequest),
									corev1.ResourceCPU:    mustParseQuantity(PostgresCPURequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity(PostgresMemoryLimit),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", PostgresDBUser},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", PostgresDBUser},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "postgres-storage",
									MountPath: "/var/lib/postgresql/data",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "postgres-storage",
						Labels: map[string]string{
							"knowledgebase": kb.Name,
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(PostgresStorageSize),
							},
						},
					},
				},
			},
		},
	}
}

func buildPostgresService(kb *platformv1alpha1.KnowledgeBase) *corev1.Service {
	labels := postgresLabels(kb)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresServiceName(kb),
			Namespace: infraNamespace(kb),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp",
					Port:       int32(PostgresPort),
					TargetPort: intstr.FromString("tcp"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
}

func (r *KnowledgeBaseReconciler) reconcilePostgres(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	ss := buildPostgresStatefulSet(kb)
	svc := buildPostgresService(kb)
	return r.reconcileWorkloadAndService(ctx, kb, ss, svc, "postgres")
}

func (r *KnowledgeBaseReconciler) cleanupPostgres(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	ss := &appsv1.StatefulSet{}
	ss.Name = postgresStatefulSetName(kb)
	ss.Namespace = infraNamespace(kb)
	svc := &corev1.Service{}
	svc.Name = postgresServiceName(kb)
	svc.Namespace = infraNamespace(kb)
	return r.cleanupWorkloadAndService(ctx, ss, svc)
}
