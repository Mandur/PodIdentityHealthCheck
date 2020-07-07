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

func TestPodShouldStartIfPodIdentityIsInstalled(t *testing.T) {

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

func TestPodShouldNotStartIfNMIIsMissing(t *testing.T) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")

	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)

	podPath := "./fixtures/podWithPIEnabled.yaml"
	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	time.Sleep(15 * time.Second)
	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Verify the pod starts
	err := k8s.WaitUntilPodAvailableE(t, options, "podidentity-test-pod", 5, 3*time.Second)
	assert.Error(t, err)
}

func TestPodShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	removeAzureIdentity(t, options)
	podPath := "./fixtures/podWithPIEnabled.yaml"
	time.Sleep(15 * time.Second)

	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Verify the pod starts
	err := k8s.WaitUntilPodAvailableE(t, options, "podidentity-test-pod", 6, 10*time.Second)
	assert.Nil(t, err)

}
