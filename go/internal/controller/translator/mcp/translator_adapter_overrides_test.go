/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcp

import (
	"testing"

	"github.com/kagent-dev/kagent/go/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestApplyPodTemplateOverrides(t *testing.T) {
	translator := &transportAdapterTranslator{}

	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "default",
				},
			},
		},
	}

	overrides := &v1alpha1.PodTemplateOverrides{
		NodeSelector: map[string]string{
			"disktype": "ssd",
			"zone":     "us-east-1a",
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "mcp-workloads",
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
		Affinity: &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					{
						Weight: 100,
						PodAffinityTerm: corev1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test",
								},
							},
							TopologyKey: "kubernetes.io/hostname",
						},
					},
				},
			},
		},
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: func() *bool { b := true; return &b }(),
			RunAsUser:    func() *int64 { i := int64(1000); return &i }(),
			FSGroup:      func() *int64 { i := int64(2000); return &i }(),
		},
		Annotations: map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   "9090",
		},
		Labels: map[string]string{
			"environment": "production",
		},
		PriorityClassName: "system-cluster-critical",
	}

	err := translator.applyPodTemplateOverrides(deployment, overrides)
	require.NoError(t, err)

	// Verify NodeSelector
	assert.Equal(t, "ssd", deployment.Spec.Template.Spec.NodeSelector["disktype"])
	assert.Equal(t, "us-east-1a", deployment.Spec.Template.Spec.NodeSelector["zone"])

	// Verify Tolerations
	assert.Len(t, deployment.Spec.Template.Spec.Tolerations, 1)
	assert.Equal(t, "dedicated", deployment.Spec.Template.Spec.Tolerations[0].Key)

	// Verify Affinity
	assert.NotNil(t, deployment.Spec.Template.Spec.Affinity)
	assert.NotNil(t, deployment.Spec.Template.Spec.Affinity.PodAntiAffinity)

	// Verify SecurityContext
	assert.NotNil(t, deployment.Spec.Template.Spec.SecurityContext)
	assert.True(t, *deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot)
	assert.Equal(t, int64(1000), *deployment.Spec.Template.Spec.SecurityContext.RunAsUser)

	// Verify Annotations (merged with existing)
	assert.Equal(t, "true", deployment.Spec.Template.Annotations["prometheus.io/scrape"])

	// Verify Labels (merged with existing)
	assert.Equal(t, "test", deployment.Spec.Template.Labels["app"])
	assert.Equal(t, "production", deployment.Spec.Template.Labels["environment"])

	// Verify PriorityClassName
	assert.Equal(t, "system-cluster-critical", deployment.Spec.Template.Spec.PriorityClassName)
}

func TestApplyContainerOverrides(t *testing.T) {
	translator := &transportAdapterTranslator{}

	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mcp-server",
							Image: "test:latest",
						},
					},
				},
			},
		},
	}

	overrides := &v1alpha1.ContainerOverrides{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
		Lifecycle: &corev1.Lifecycle{
			PostStart: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "echo Started"},
				},
			},
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "sleep 15"},
				},
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: func() *bool { b := false; return &b }(),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(3000),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ready",
					Port: intstr.FromInt(3000),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
		ImagePullPolicy: corev1.PullAlways,
	}

	err := translator.applyContainerOverrides(deployment, overrides)
	require.NoError(t, err)

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify Resources
	assert.Equal(t, "100m", container.Resources.Requests.Cpu().String())
	assert.Equal(t, "128Mi", container.Resources.Requests.Memory().String())
	assert.Equal(t, "500m", container.Resources.Limits.Cpu().String())
	assert.Equal(t, "512Mi", container.Resources.Limits.Memory().String())

	// Verify Lifecycle
	assert.NotNil(t, container.Lifecycle)
	assert.NotNil(t, container.Lifecycle.PostStart)
	assert.Equal(t, "/bin/sh", container.Lifecycle.PostStart.Exec.Command[0])

	// Verify SecurityContext
	assert.NotNil(t, container.SecurityContext)
	assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)

	// Verify Probes
	assert.NotNil(t, container.LivenessProbe)
	assert.Equal(t, "/healthz", container.LivenessProbe.HTTPGet.Path)
	assert.NotNil(t, container.ReadinessProbe)
	assert.Equal(t, "/ready", container.ReadinessProbe.HTTPGet.Path)

	// Verify ImagePullPolicy
	assert.Equal(t, corev1.PullAlways, container.ImagePullPolicy)
}

func TestApplyDeploymentOverrides(t *testing.T) {
	translator := &transportAdapterTranslator{}

	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: func() *int32 { i := int32(1); return &i }(),
		},
	}

	overrides := &v1alpha1.DeploymentOverrides{
		Replicas: func() *int32 { i := int32(3); return &i }(),
		Strategy: &appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
			},
		},
		MinReadySeconds:      10,
		RevisionHistoryLimit: func() *int32 { i := int32(5); return &i }(),
	}

	err := translator.applyDeploymentOverrides(deployment, overrides)
	require.NoError(t, err)

	// Verify Replicas
	assert.Equal(t, int32(3), *deployment.Spec.Replicas)

	// Verify Strategy
	assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, deployment.Spec.Strategy.Type)
	assert.Equal(t, int32(1), deployment.Spec.Strategy.RollingUpdate.MaxSurge.IntVal)
	assert.Equal(t, int32(0), deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal)

	// Verify MinReadySeconds
	assert.Equal(t, int32(10), deployment.Spec.MinReadySeconds)

	// Verify RevisionHistoryLimit
	assert.Equal(t, int32(5), *deployment.Spec.RevisionHistoryLimit)
}

func TestApplyOverrides_NilOverrides(t *testing.T) {
	translator := &transportAdapterTranslator{}

	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mcp-server",
							Image: "test:latest",
						},
					},
				},
			},
		},
	}

	// Test with nil overrides - should not error
	err := translator.applyPodTemplateOverrides(deployment, nil)
	assert.NoError(t, err)

	err = translator.applyContainerOverrides(deployment, nil)
	assert.NoError(t, err)

	err = translator.applyDeploymentOverrides(deployment, nil)
	assert.NoError(t, err)
}
