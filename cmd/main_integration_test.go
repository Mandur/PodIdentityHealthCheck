package main

import (
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
)

func setupPodIdentity(t *testing.T, options *k8s.KubectlOptions) {
	podIdentityPath := "https://raw.githubusercontent.com/Azure/aad-pod-identity/master/deploy/infra/deployment-rbac.yaml"
	k8s.KubectlApply(t, options, podIdentityPath)
}

func setupAzureIdentity(t *testing.T, options *k8s.KubectlOptions) {
	identityPath := "./fixtures/identity.yaml"
	k8s.KubectlApply(t, options, identityPath)
}

func removeAzureIdentity(t *testing.T, options *k8s.KubectlOptions) {
	identityPath := "./fixtures/identity.yaml"
	k8s.KubectlDeleteE(t, options, identityPath)
}

func checkIfPodRestarted(t *testing.T, options *k8s.KubectlOptions, podName string) bool {
	pod := k8s.GetPod(t, options, podName)
	if pod.Status.ContainerStatuses[0].RestartCount > 0 {
		return true
	}

	return false
}

func checkIfPodReady(t *testing.T, options *k8s.KubectlOptions, podName string) bool {
	pod := k8s.GetPod(t, options, podName)
	return pod.Status.ContainerStatuses[0].Ready
}

func TestCustomPodShouldStartIfPodIdentityIsInstalled(t *testing.T) {

	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)
	podPath := "./fixtures/podWithPIEnabled.yaml"

	defer k8s.KubectlDelete(t, options, podPath)
	k8s.KubectlApply(t, options, podPath)

	// Verify the pod starts
	err := k8s.WaitUntilPodAvailableE(t, options, "podidentity-test-pod", 6, 10*time.Second)
	assert.Nil(t, err)
}

func TestCustomPodShouldStopIfNMIIsMissing(t *testing.T) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")

	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)
	podName := "podidentity-test-pod"
	podPath := "./fixtures/podWithPIEnabled.yaml"
	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Verify the pod starts
	k8s.WaitUntilPodAvailable(t, options, podName, 10, 3*time.Second)
	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	checkCount := 0
	for !checkIfPodRestarted(t, options, podName) {
		if checkCount > 5 {
			assert.Fail(t, "The pod did not restart after the nmi indicated failure")
		}
		checkCount++
		time.Sleep(10 * time.Second)
	}
}

func TestCustomPodShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	removeAzureIdentity(t, options)
	podPath := "./fixtures/podWithPIEnabled.yaml"
	podName := "podidentity-test-pod"

	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Verify the pod starts
	k8s.WaitUntilPodAvailableE(t, options, podName, 6, 10*time.Second)
	checkCount := 0
	for !checkIfPodRestarted(t, options, podName) {
		if checkCount > 5 {
			assert.Fail(t, "The pod did not restart after the nmi indicated failure")
		}
		checkCount++
		time.Sleep(5 * time.Second)
	}
}

func TestYamlPodShouldStartIfPodIdentityIsInstalled(t *testing.T) {

	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)
	podPath := "./fixtures/podwithYaml.yaml"

	defer k8s.KubectlDelete(t, options, podPath)
	k8s.KubectlApply(t, options, podPath)

	// Verify the pod starts
	err := k8s.WaitUntilPodAvailableE(t, options, "podidentit-yaml-test-pod", 6, 10*time.Second)
	assert.Nil(t, err)
	assert.True(t, checkIfPodReady(t, options, "podidentit-yaml-test-pod"))

}

func TestYamlPodShouldStopIfNMIIsMissing(t *testing.T) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")

	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)

	podPath := "./fixtures/podwithYaml.yaml"
	podName := "podidentit-yaml-test-pod"
	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Verify the pod starts
	k8s.WaitUntilPodAvailable(t, options, podName, 60, 1*time.Second)
	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	time.Sleep(30 * time.Second)
	checkCount := 0
	for !checkIfPodRestarted(t, options, podName) {
		assert.False(t, checkIfPodReady(t, options, podName))
		if checkCount > 5 {
			assert.Fail(t, "The pod did not restart after the nmi indicated failure")
		}
		checkCount++
		time.Sleep(20 * time.Second)
	}
}

func TestYamlPodShouldNotBeReadyIfNMIIsMissing(t *testing.T) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	podPath := "./fixtures/podwithYaml.yaml"
	podName := "podidentit-yaml-test-pod"

	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Verify the pod starts
	k8s.WaitUntilPodAvailable(t, options, podName, 20, 1*time.Second)
	checkCount := 0
	for !checkIfPodRestarted(t, options, podName) {
		assert.False(t, checkIfPodReady(t, options, podName))
		if checkCount > 10 {
			assert.Fail(t, "The pod did not restart after the nmi indicated failure")
		}
		checkCount++
		time.Sleep(10 * time.Second)
	}
}
