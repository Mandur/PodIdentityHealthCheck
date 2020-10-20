package main

import (
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
)

const (
	podName                           = "podidentity-test-pod"
	nmiHealthCheckInProbesPath        = "./fixtures/nmiHealthCheckInProbes.yaml"
	nmiHealthCheckInCodePath          = "./fixtures/nmiHealthCheckInCode.yaml"
	nmiHealthCheckInFullProbesPath    = "./fixtures/identityHealthCheckInHealthProbes.yaml"
	identityCheckInInitContainerPath  = "./fixtures/identityCheckInInitContainer.yaml"
	identityCheckInSidecarProbesPath  = "./fixtures/identityHealthCheckInSidecarProbes.yaml"
	nmiHealthCheckInInitContainerPath = "./fixtures/nmiHealthCheckInInitContainer.yaml"
	identityHealthCheckInCode         = "./fixtures/identityHealthCheckInCode.yaml"
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

func ensureApplicationPodIsDeleted(t *testing.T, options *k8s.KubectlOptions, podname string, yamlPath string) {
	for {
		_, err := k8s.GetPodE(t, options, podName)
		if err != nil {
			break
		}

		k8s.KubectlDeleteE(t, options, yamlPath)
	}
}

func ensureNMIPodsAreDeleted(t *testing.T, options *k8s.KubectlOptions) {
	for {
		time.Sleep(5 * time.Second)
		pod, _ := k8s.RunKubectlAndGetOutputE(t, options, "get", "pods", "-l", "component=nmi")
		if pod == "No resources found in default namespace." {
			break
		}

		k8s.RunKubectlAndGetOutputE(t, options, "delete", "pods", "-l", "component=nmi")
	}
}

func checkIfPodReady(t *testing.T, options *k8s.KubectlOptions, podName string) bool {
	pod := k8s.GetPod(t, options, podName)
	if len(pod.Status.ContainerStatuses) > 0 {
		return pod.Status.ContainerStatuses[0].Ready
	}

	return false
}

func CheckPodShouldNotStartIfAzureIdentityIsMissing(t *testing.T, podPath string, expectSuccess bool) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	removeAzureIdentity(t, options)
	// delete the nmi to avoid identity caching
	k8s.RunKubectlAndGetOutputE(t, options, "delete", "pods", "-l", "component=nmi")
	ensureApplicationPodIsDeleted(t, options, podName, podPath)
	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)

	// Assert the pod started correctly
	if expectSuccess {
		for !checkIfPodReady(t, options, podName) {
			time.Sleep(5 * time.Second)
		}
	} else {
		time.Sleep(10 * time.Second)

	}

	checkCount := 0
	if expectSuccess {
		for !checkIfPodRestarted(t, options, podName) {
			assert.False(t, checkIfPodReady(t, options, podName))
			if checkCount > 10 {
				assert.Fail(t, "The pod did not restart after the nmi indicated failure")
			}
			checkCount++
			time.Sleep(10 * time.Second)
		}
	} else {
		for i := 0; i < 5; i++ {
			assert.False(t, checkIfPodRestarted(t, options, podName))
			assert.True(t, checkIfPodReady(t, options, podName))
			time.Sleep(5 * time.Second)
		}
	}
}

func CheckPodStartCorrectly(t *testing.T, podPath string) {
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)
	ensureApplicationPodIsDeleted(t, options, podName, podPath)

	defer k8s.KubectlDelete(t, options, podPath)
	k8s.KubectlApply(t, options, podPath)

	// Verify the pod starts
	err := k8s.WaitUntilPodAvailableE(t, options, podName, 6, 10*time.Second)
	assert.Nil(t, err)
	assert.True(t, checkIfPodReady(t, options, podName))
}

func CheckPodShouldStopIfNMIFails(t *testing.T, podPath string, expectSuccess bool) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")

	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)
	ensureApplicationPodIsDeleted(t, options, podName, podPath)

	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Assert the pod started correctly
	if expectSuccess {
		for !checkIfPodReady(t, options, podName) {
			time.Sleep(5 * time.Second)
		}
	} else {
		time.Sleep(10 * time.Second)

	}
	// Delete the NMI to simulate a shutdown
	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	ensureNMIPodsAreDeleted(t, options)

	checkCount := 0
	if expectSuccess {
		// Assert that the pod is getting restarted. While the pod is still alive the pod should be tagged as not ready.
		for !checkIfPodRestarted(t, options, podName) {
			assert.False(t, checkIfPodReady(t, options, podName))
			if checkCount > 20 {
				assert.FailNow(t, "The pod did not restart after the nmi indicated failure")
			}
			checkCount++
			time.Sleep(5 * time.Second)
		}
	} else {
		// in this case we expect the pod to keep running
		for i := 0; i < 5; i++ {
			assert.False(t, checkIfPodRestarted(t, options, podName))
			assert.True(t, checkIfPodReady(t, options, podName))
			time.Sleep(5 * time.Second)
		}
	}
}

func CheckPodShouldNeverBeReadyIfNMIIsNotReady(t *testing.T, podPath string, expectSuccess bool) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")

	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)
	ensureApplicationPodIsDeleted(t, options, podName, podPath)

	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	ensureNMIPodsAreDeleted(t, options)

	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// Assert the pod started correctly
	if expectSuccess {
		for !checkIfPodReady(t, options, podName) {
			time.Sleep(5 * time.Second)
		}
	} else {
		time.Sleep(10 * time.Second)

	}
	checkCount := 0
	// we loop until there is at least one restart, we also check that the pod is never ready to accept traffic
	for !checkIfPodRestarted(t, options, podName) {
		assert.False(t, checkIfPodReady(t, options, podName))
		if checkCount > 15 {
			assert.Fail(t, "The pod did not restart after the nmi indicated failure")
		}
		checkCount++
		time.Sleep(5 * time.Second)
	}
}

func CheckPodShouldBePreventedToStartIfNMIIsNotReady(t *testing.T, podPath string, expectSuccess bool) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")

	setupPodIdentity(t, options)
	setupAzureIdentity(t, options)
	ensureApplicationPodIsDeleted(t, options, podName, podPath)

	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	ensureNMIPodsAreDeleted(t, options)
	defer k8s.KubectlDelete(t, options, podPath)
	// Assert the pod started correctly
	if expectSuccess {
		for !checkIfPodReady(t, options, podName) {
			time.Sleep(5 * time.Second)
		}
	} else {
		time.Sleep(10 * time.Second)

	}
	checkCount := 0
	// we assert that the pod never get ready
	for i := 0; i < 5; i++ {
		assert.False(t, checkIfPodReady(t, options, podName))
		if checkCount > 10 {
			assert.Fail(t, "The pod did not restart after the nmi indicated failure")
		}
		checkCount++
		time.Sleep(5 * time.Second)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI Health check as part of application probes
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Happy case, pod start correctly if NMI is setup correctly
func TestNMIHealthCheckPodShouldStartCorrectlyIfPodIdentityIsInstalled(t *testing.T) {
	CheckPodStartCorrectly(t, nmiHealthCheckInProbesPath)
}

// If the NMI fails or stops, the pods probe should graciously terminate it.
func TestNMIHealthCheckPodShouldStopIfNMIFails(t *testing.T) {
	CheckPodShouldStopIfNMIFails(t, nmiHealthCheckInProbesPath, true)
}

// If the NMI is missing, the probes should prevent the pod to start.
func TestNMIHealthCheckPodShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	CheckPodShouldNeverBeReadyIfNMIIsNotReady(t, nmiHealthCheckInProbesPath, true)
}

// If the Azure Identity is missing, this method does not permit to check this.
func TestPodShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
	CheckPodShouldNotStartIfAzureIdentityIsMissing(t, nmiHealthCheckInProbesPath, false)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI Health check within application code
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Happy case, pod start correctly if NMI is setup correctly
func TestNMIHealthCheckCustomPodShouldStartCorrectlyIfPodIdentityIsInstalled(t *testing.T) {
	CheckPodStartCorrectly(t, nmiHealthCheckInCodePath)
}

// If the NMI fails or stops, the pods probe should graciously terminate it.
func TestNMIHealthCheckCustomPodShouldStopIfNMIFails(t *testing.T) {
	CheckPodShouldStopIfNMIFails(t, nmiHealthCheckInCodePath, true)
}

// If the NMI is missing, the probes should prevent the pod to start.
func TestNMIHealthCheckCustomPodShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	CheckPodShouldNeverBeReadyIfNMIIsNotReady(t, nmiHealthCheckInCodePath, true)
}

// If the Azure Identity is missing, this method does not permit to check this. So we expect that even if the azure identity is not here the pod will start.
func TestCustomPodWillStartEvenIfAzureIdentityIsMissing(t *testing.T) {
	CheckPodShouldNotStartIfAzureIdentityIsMissing(t, nmiHealthCheckInCodePath, false)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full pod identity in init container
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Happy case, pod start correctly if NMI is setup correctly
func TestIdentityCheckInInitContainerShouldStartCorrectlyIfPodIdentityIsInstalled(t *testing.T) {
	CheckPodStartCorrectly(t, identityCheckInInitContainerPath)
}

// If the NMI fails or stops, an init container is unable to stop a running pod
func TestIdentityCheckInInitContainerShouldStopIfNMIFails(t *testing.T) {
	CheckPodShouldStopIfNMIFails(t, identityCheckInInitContainerPath, false)
}

// If the NMI is missing, the init container should prevent the pod to start.
func TestIdentityCheckInInitContainerShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	CheckPodShouldBePreventedToStartIfNMIIsNotReady(t, identityCheckInInitContainerPath, true)
}

// If the Azure Identity is missing, this method prevent the pod to start
func TestIdentityCheckInInitContainerShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
	CheckPodShouldNotStartIfAzureIdentityIsMissing(t, identityCheckInInitContainerPath, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI probes health check in container init
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Happy case, pod start correctly if NMI is setup correctly
func TestNmiHealthCheckInInitPodShouldStartCorrectlyIfPodIdentityIsInstalled(t *testing.T) {
	CheckPodStartCorrectly(t, nmiHealthCheckInInitContainerPath)
}

// If the NMI fails or stops, there is no way an init container could stop it.
func TestNmiHealthCheckInInitPodShouldStopIfNMIFails(t *testing.T) {
	CheckPodShouldStopIfNMIFails(t, nmiHealthCheckInInitContainerPath, false)
}

// If the NMI is missing, the probes should prevent the pod to start.
func TestNmiHealthCheckInInitPodShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	CheckPodShouldBePreventedToStartIfNMIIsNotReady(t, nmiHealthCheckInInitContainerPath, false)
}

// If the Azure Identity is missing, this method does not permit to check this.
func TestNmiHealthCheckInInitPodShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
	CheckPodShouldNotStartIfAzureIdentityIsMissing(t, nmiHealthCheckInInitContainerPath, false)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full identity check as sidecar container
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Happy case, pod start correctly if NMI is setup correctly
func TestIdentityHealthCheckInSidecarShouldStartCorrectlyIfPodIdentityIsInstalled(t *testing.T) {
	CheckPodStartCorrectly(t, identityCheckInSidecarProbesPath)
}

// If the NMI fails or stops, there is no way an init container could stop it.
func TestIdentityHealthCheckInSidecarShouldStopIfNMIFails(t *testing.T) {
	CheckPodShouldStopIfNMIFails(t, identityCheckInSidecarProbesPath, false)
}

// If the NMI is missing, the az cli will have cached the credentials therefore there is no way to stop the already pod.
func TestIdentityHealthCheckInSidecarShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	CheckPodShouldBePreventedToStartIfNMIIsNotReady(t, identityCheckInSidecarProbesPath, false)
}

// If the Azure Identity is missing, in theory we could see the sidecar stopping the pod, however this does not work as az cli will have cached the id.
func TestIdentityHealthCheckInSidecarShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
	CheckPodShouldNotStartIfAzureIdentityIsMissing(t, identityCheckInSidecarProbesPath, false)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full identity check as part of the Health Checks (preferred method)
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Happy case, pod start correctly if NMI is setup correctly
func TestIdentityHealthCheckInHealthProbesShouldStartCorrectlyIfPodIdentityIsInstalled(t *testing.T) {
	CheckPodStartCorrectly(t, nmiHealthCheckInFullProbesPath)
}

// If the NMI fails or stops, the health probe should stop it
func TestIdentityHealthCheckInHealthProbesShouldStopIfNMIFails(t *testing.T) {
	CheckPodShouldStopIfNMIFails(t, nmiHealthCheckInFullProbesPath, true)
}

// If the NMI is missing, the health probe should catch it and restart the pod
func TestIdentityHealthCheckInHealthProbesShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	CheckPodShouldBePreventedToStartIfNMIIsNotReady(t, nmiHealthCheckInFullProbesPath, true)
}

// If the Azure Identity is missing, the health probe will fail acquire a token and restart the pod
func TestIdentityHealthCheckInHealthProbesShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
	CheckPodShouldNotStartIfAzureIdentityIsMissing(t, nmiHealthCheckInFullProbesPath, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full identity check as part of the code
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Happy case, pod start correctly if NMI is setup correctly
// func TestIdentityHealthCheckInCodeShouldStartCorrectlyIfPodIdentityIsInstalled(t *testing.T) {
// 	CheckPodStartCorrectly(t, identityHealthCheckInCode)
// }

// // If the NMI fails or stops, there is no way an init container could stop it.
// func TestIdentityHealthCheckInCodeShouldStopIfNMIFails(t *testing.T) {
// 	CheckPodShouldStopIfNMIFails(t, identityHealthCheckInCode, true)
// }

// // If the NMI is missing, the az cli will have cached the credentials therefore there is no way to stop the already pod.
// func TestIdentityHealthCheckInCodeShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
// 	CheckPodShouldBePreventedToStartIfNMIIsNotReady(t, identityHealthCheckInCode, true)
// }

// // If the Azure Identity is missing, in theory we could see the sidecar stopping the pod, however this does not work as az cli will have cached the id.
// func TestIdentityHealthCheckInCodeShouldNotStartIfAzureIdentityIsMissing(t *testing.T) {
// 	CheckPodShouldNotStartIfAzureIdentityIsMissing(t, identityHealthCheckInCode, true)
// }
