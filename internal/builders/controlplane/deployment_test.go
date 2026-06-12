// Copyright 2022 Clastix Labs
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	kamajiv1alpha1 "github.com/clastix/kamaji/api/v1alpha1"
)

func schedulerContainerIndex(podSpec corev1.PodSpec) int {
	for i, c := range podSpec.Containers {
		if c.Name == schedulerContainerName {
			return i
		}
	}

	return -1
}

func controllerManagerContainerIndex(podSpec corev1.PodSpec) int {
	for i, c := range podSpec.Containers {
		if c.Name == controlPlaneContainerName {
			return i
		}
	}

	return -1
}

var _ = Describe("Controlplane Deployment", func() {
	var d Deployment
	BeforeEach(func() {
		d = Deployment{
			DataStoreOverrides: []DataStoreOverrides{{
				Resource: "/events",
				DataStore: kamajiv1alpha1.DataStore{
					Spec: kamajiv1alpha1.DataStoreSpec{
						Endpoints: kamajiv1alpha1.Endpoints{"etcd-0", "etcd-1", "etcd-2"},
					},
				},
			}},
		}
	})

	Describe("schedulerStartupInitialDelay", func() {
		It("returns 0 by default when no env var is set", func() {
			Expect(schedulerStartupInitialDelay(nil)).To(Equal(int32(0)))
		})

		It("returns the value parsed from the env var", func() {
			envVars := []corev1.EnvVar{
				{Name: schedulerStartupInitialDelayEnvVar, Value: "30"},
			}
			Expect(schedulerStartupInitialDelay(envVars)).To(Equal(int32(30)))
		})

		It("falls back to default when the env var value is not a valid integer", func() {
			envVars := []corev1.EnvVar{
				{Name: schedulerStartupInitialDelayEnvVar, Value: "not-a-number"},
			}
			Expect(schedulerStartupInitialDelay(envVars)).To(Equal(int32(0)))
		})

		It("ignores unrelated env vars", func() {
			envVars := []corev1.EnvVar{
				{Name: "UNRELATED_VAR", Value: "99"},
			}
			Expect(schedulerStartupInitialDelay(envVars)).To(Equal(int32(0)))
		})
	})

	Describe("controllerManagerStartupInitialDelay", func() {
		It("returns 0 by default when no env var is set", func() {
			Expect(controllerManagerStartupInitialDelay(nil)).To(Equal(int32(0)))
		})

		It("returns the value parsed from the env var", func() {
			envVars := []corev1.EnvVar{
				{Name: controllerManagerStartupInitialDelayEnvVar, Value: "45"},
			}
			Expect(controllerManagerStartupInitialDelay(envVars)).To(Equal(int32(45)))
		})

		It("falls back to default when the env var value is not a valid integer", func() {
			envVars := []corev1.EnvVar{
				{Name: controllerManagerStartupInitialDelayEnvVar, Value: "bad-value"},
			}
			Expect(controllerManagerStartupInitialDelay(envVars)).To(Equal(int32(0)))
		})

		It("ignores unrelated env vars", func() {
			envVars := []corev1.EnvVar{
				{Name: "UNRELATED_VAR", Value: "99"},
			}
			Expect(controllerManagerStartupInitialDelay(envVars)).To(Equal(int32(0)))
		})
	})

	Describe("buildScheduler", func() {
		var (
			podSpec corev1.PodSpec
			tcp     kamajiv1alpha1.TenantControlPlane
		)

		BeforeEach(func() {
			podSpec = corev1.PodSpec{}
			tcp = kamajiv1alpha1.TenantControlPlane{}
			tcp.Spec.Kubernetes.Version = "v1.29.0"
		})

		It("sets no env vars on the scheduler container when AdditionalEnv is nil", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = nil
			d.buildScheduler(&podSpec, tcp)

			idx := schedulerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].Env).To(BeEmpty())
		})

		It("propagates AdditionalEnv.Scheduler onto the container", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = &kamajiv1alpha1.ControlPlaneAdditionalEnv{
				Scheduler: []corev1.EnvVar{
					{Name: "MY_VAR", Value: "hello"},
				},
			}
			d.buildScheduler(&podSpec, tcp)

			idx := schedulerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].Env).To(ContainElement(corev1.EnvVar{Name: "MY_VAR", Value: "hello"}))
		})

		It("uses the startup delay env var as StartupProbe InitialDelaySeconds", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = &kamajiv1alpha1.ControlPlaneAdditionalEnv{
				Scheduler: []corev1.EnvVar{
					{Name: schedulerStartupInitialDelayEnvVar, Value: "25"},
				},
			}
			d.buildScheduler(&podSpec, tcp)

			idx := schedulerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].StartupProbe).NotTo(BeNil())
			Expect(podSpec.Containers[idx].StartupProbe.InitialDelaySeconds).To(Equal(int32(25)))
		})

		It("uses default InitialDelaySeconds in StartupProbe when AdditionalEnv is nil", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = nil
			d.buildScheduler(&podSpec, tcp)

			idx := schedulerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].StartupProbe).NotTo(BeNil())
			Expect(podSpec.Containers[idx].StartupProbe.InitialDelaySeconds).To(Equal(schedulerStartupInitialDelaySecondsDefault))
		})
	})

	Describe("buildControllerManager", func() {
		var (
			podSpec corev1.PodSpec
			tcp     kamajiv1alpha1.TenantControlPlane
		)

		BeforeEach(func() {
			podSpec = corev1.PodSpec{}
			tcp = kamajiv1alpha1.TenantControlPlane{}
			tcp.Spec.Kubernetes.Version = "v1.29.0"
			tcp.Spec.NetworkProfile.ServiceCIDR = "10.96.0.0/12"
			tcp.Spec.NetworkProfile.PodCIDR = "10.244.0.0/16"
		})

		It("sets no env vars on the controller-manager container when AdditionalEnv is nil", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = nil
			d.buildControllerManager(&podSpec, tcp)

			idx := controllerManagerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].Env).To(BeEmpty())
		})

		It("propagates AdditionalEnv.ControllerManager onto the container", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = &kamajiv1alpha1.ControlPlaneAdditionalEnv{
				ControllerManager: []corev1.EnvVar{
					{Name: "MY_CM_VAR", Value: "world"},
				},
			}
			d.buildControllerManager(&podSpec, tcp)

			idx := controllerManagerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].Env).To(ContainElement(corev1.EnvVar{Name: "MY_CM_VAR", Value: "world"}))
		})

		It("uses the startup delay env var as StartupProbe InitialDelaySeconds", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = &kamajiv1alpha1.ControlPlaneAdditionalEnv{
				ControllerManager: []corev1.EnvVar{
					{Name: controllerManagerStartupInitialDelayEnvVar, Value: "40"},
				},
			}
			d.buildControllerManager(&podSpec, tcp)

			idx := controllerManagerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].StartupProbe).NotTo(BeNil())
			Expect(podSpec.Containers[idx].StartupProbe.InitialDelaySeconds).To(Equal(int32(40)))
		})

		It("uses default InitialDelaySeconds in StartupProbe when AdditionalEnv is nil", func() {
			tcp.Spec.ControlPlane.Deployment.AdditionalEnv = nil
			d.buildControllerManager(&podSpec, tcp)

			idx := controllerManagerContainerIndex(podSpec)
			Expect(podSpec.Containers[idx].StartupProbe).NotTo(BeNil())
			Expect(podSpec.Containers[idx].StartupProbe.InitialDelaySeconds).To(Equal(controllerManagerStartupInitialDelaySecondsDefault))
		})
	})

	Describe("DataStoreOverrides flag generation", func() {
		It("should generate valid --etcd-servers-overrides value", func() {
			etcdSerVersOverrides := d.etcdServersOverrides()
			Expect(etcdSerVersOverrides).To(Equal("/events#https://etcd-0;https://etcd-1;https://etcd-2"))
		})
		It("should generate valid --etcd-servers-overrides value with 2 DataStoreOverrides", func() {
			d.DataStoreOverrides = append(d.DataStoreOverrides, DataStoreOverrides{
				Resource: "/pods",
				DataStore: kamajiv1alpha1.DataStore{
					Spec: kamajiv1alpha1.DataStoreSpec{
						Endpoints: kamajiv1alpha1.Endpoints{"etcd-3", "etcd-4", "etcd-5"},
					},
				},
			})
			etcdSerVersOverrides := d.etcdServersOverrides()
			Expect(etcdSerVersOverrides).To(Equal("/events#https://etcd-0;https://etcd-1;https://etcd-2,/pods#https://etcd-3;https://etcd-4;https://etcd-5"))
		})
	})
})
