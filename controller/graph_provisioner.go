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
	logger := log.FromContext(ctx)

	deploy := buildGraphDeployment(kb)
	if err := controllerutil.SetControllerReference(kb, deploy, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on graph deployment: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deploy.Name, Namespace: deploy.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, deploy); err != nil {
				return fmt.Errorf("failed to create graph deployment: %w", err)
			}
			logger.Info("Created Graph Deployment", "name", deploy.Name)
		} else {
			return fmt.Errorf("failed to get graph deployment: %w", err)
		}
	} else {
		deploy.ResourceVersion = existing.ResourceVersion
		if err := r.Update(ctx, deploy); err != nil {
			return fmt.Errorf("failed to update graph deployment: %w", err)
		}
	}

	svc := buildGraphService(kb)
	if err := controllerutil.SetControllerReference(kb, svc, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on graph service: %w", err)
	}

	existingSvc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, existingSvc)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, svc); err != nil {
				return fmt.Errorf("failed to create graph service: %w", err)
			}
			logger.Info("Created Graph Service", "name", svc.Name)
		} else {
			return fmt.Errorf("failed to get graph service: %w", err)
		}
	} else {
		svc.ResourceVersion = existingSvc.ResourceVersion
		svc.Spec.ClusterIP = existingSvc.Spec.ClusterIP
		if err := r.Update(ctx, svc); err != nil {
			return fmt.Errorf("failed to update graph service: %w", err)
		}
	}

	return nil
}

func (r *KnowledgeBaseReconciler) cleanupGraph(ctx context.Context, kb *platformv1alpha1.KnowledgeBase) error {
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Name: graphDeploymentName(kb), Namespace: kb.Namespace}, deploy); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get graph deployment for cleanup: %w", err)
	}
	if err := r.Delete(ctx, deploy); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete graph deployment: %w", err)
	}

	svc := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Name: graphServiceName(kb), Namespace: kb.Namespace}, svc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get graph service for cleanup: %w", err)
	}
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete graph service: %w", err)
	}

	return nil
}
