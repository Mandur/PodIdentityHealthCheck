package main

import (
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
)

const (
	podName          = "podidentity-test-pod"
	podNameWithNoTag = "podidentity-test-pod-with-no-tags"
)

func setupPodIdentity(t *testing.T, options *k8s.KubectlOptions) {
	podIdentityPath := "https://raw.githubusercontent.com/Azure/aad-pod-identity/master/deploy/infra/deployment-rbac.yaml"
	k8s.KubectlApply(t, options, podIdentityPath)
}

func checkIfPodRestarted(t *testing.T, options *k8s.KubectlOptions, podName string) bool {
	pod := k8s.GetPod(t, options, podName)
	if pod.Status.ContainerStatuses[0].RestartCount > 0 {
		return true
	}

	return false
}

func assertPodKeepRunningAndDontRestart(t *testing.T, options *k8s.KubectlOptions, podName string) {
	for i := 0; i < 5; i++ {
		assert.False(t, checkIfPodRestarted(t, options, podName))
		assert.True(t, checkIfPodReady(t, options, podName))
		time.Sleep(5 * time.Second)
	}
}

func checkPodRestartAndStayNotReady(t *testing.T, options *k8s.KubectlOptions, podName string) {
	checkCount := 0
	for !checkIfPodRestarted(t, options, podName) {
		assert.False(t, checkIfPodReady(t, options, podName))
		if checkCount > 10 {
			assert.FailNow(t, "The pod did not restart after the nmi indicated failure")
		}
		checkCount++
		time.Sleep(10 * time.Second)
	}
}

func checkPodStayNotReady(t *testing.T, options *k8s.KubectlOptions, podName string) {
	for i := 0; i < 5; i++ {
		assert.False(t, checkIfPodReady(t, options, podName))
		time.Sleep(5 * time.Second)
	}
}

func ensureApplicationPodIsDeleted(t *testing.T, options *k8s.KubectlOptions, podname string, yamlPath string) {
	//make sure to kill pod with no tag
	k8s.RunKubectlE(t, options, "delete", "pod", podNameWithNoTag)
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

// CheckPodStartCorrectly starts a pod and check it starts correctly
func checkPodStartCorrectly(t *testing.T, podPath string, killApplicationPodAfterTest bool) {
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	ensureApplicationPodIsDeleted(t, options, podName, podPath)
	if killApplicationPodAfterTest {
		defer k8s.KubectlDelete(t, options, podPath)
	}
	k8s.KubectlApply(t, options, podPath)

	// Verify the pod starts
	err := k8s.WaitUntilPodAvailableE(t, options, podName, 6, 10*time.Second)
	assert.Nil(t, err)
	// wait until pod is available
	loopCount := 0
	for !checkIfPodReady(t, options, podName) {
		time.Sleep(5 * time.Second)
		loopCount++
		if loopCount > 8 {
			assert.FailNow(t, "timeout waiting for the pod to become available")
		}
	}
	//assert pod stays available for 10 sec
	for i := 0; i < 2; i++ {
		assert.True(t, checkIfPodReady(t, options, podName))
		time.Sleep(5 * time.Second)
	}
}

// DetectIdentityStackIsNotHealthyAsNoIdentityMountedOnIMDS checks that a pod missing the identity label can't access the identity
func detectIdentityStackIsNotHealthyAsNoIdentityMountedOnIMDS(t *testing.T, podPath string, isInitContainer bool, decommisionNMI bool, expectSuccess bool) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")
	setupPodIdentity(t, options)
	ensureApplicationPodIsDeleted(t, options, podName, podPath)
	// start the pod normally to ensure identity in assigned on the node
	checkPodStartCorrectly(t, podPath, false)
	defer k8s.KubectlDelete(t, options, podPath)
	if decommisionNMI {
		options := k8s.NewKubectlOptions("", "", "default")
		// Delete the NMI to simulate a shutdown
		k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
		ensureNMIPodsAreDeleted(t, options)
	}

	podYaml, err := ioutil.ReadFile(podPath)
	if err != nil {
		assert.FailNow(t, "the configuration file could not be processed")
	}
	// we remove the labels to remove identity assignement
	podYamlString := string(podYaml)
	podYamlString = strings.ReplaceAll(podYamlString, podName, podNameWithNoTag)
	podYamlString = strings.ReplaceAll(podYamlString, "labels:", "")
	podYamlString = strings.ReplaceAll(podYamlString, "aadpodidbinding: podidentity", "")
	k8s.KubectlApplyFromStringE(t, options, podYamlString)
	defer k8s.KubectlDeleteFromStringE(t, options, podYamlString)

	// if we expect success , we expect the pod to not start
	if expectSuccess {
		time.Sleep(10 * time.Second)
		if isInitContainer {
			checkPodStayNotReady(t, options, podNameWithNoTag)
		} else {
			checkPodRestartAndStayNotReady(t, options, podNameWithNoTag)
		}
	} else {
		for !checkIfPodReady(t, options, podNameWithNoTag) {
			time.Sleep(5 * time.Second)
		}
		assertPodKeepRunningAndDontRestart(t, options, podNameWithNoTag)
	}
}

// PodIsRejectedWhenNoLabelWhenNMIIsUp Assess that a pod is flagged as not healthy by the probes if the NMI is up
func PodIsRejectedWhenNoLabelWhenNMIIsUp(t *testing.T, podPath string, isInitContainer bool, expectSuccess bool) {
	detectIdentityStackIsNotHealthyAsNoIdentityMountedOnIMDS(t, podPath, isInitContainer, false, expectSuccess)
}

// PodIsRejectedWhenNoLabelWhenNMIIsDown Assess that a pod is flagged as not healthy by the probes if the NMI is down
func PodIsRejectedWhenNoLabelWhenNMIIsDown(t *testing.T, podPath string, isInitContainer bool, expectSuccess bool) {
	detectIdentityStackIsNotHealthyAsNoIdentityMountedOnIMDS(t, podPath, isInitContainer, true, expectSuccess)
}

// DetectNMIFailsAndMakePodUnhealthy checks that if there is an NMI failure, the pod becomes not ready and restarts
func DetectNMIFailsAndMakePodUnhealthy(t *testing.T, podPath string, expectSuccess bool) {
	// start the pod normally
	checkPodStartCorrectly(t, podPath, false)
	options := k8s.NewKubectlOptions("", "", "default")
	defer k8s.KubectlDelete(t, options, podPath)
	// Delete the NMI to simulate a shutdown
	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	ensureNMIPodsAreDeleted(t, options)

	if expectSuccess {
		// Assert that the pod is getting restarted. While the pod is still alive the pod should be tagged as not ready.
		checkPodRestartAndStayNotReady(t, options, podName)

	} else {
		assertPodKeepRunningAndDontRestart(t, options, podName)
	}
}

//DetectNMIIsNotReadyAndEnsurePodIsNotReady checks that if the NMI is not ready the pod cannot accept traffic
func DetectNMIIsNotReadyAndEnsurePodIsNotReady(t *testing.T, podPath string, isInitContainer bool, expectSuccess bool) {
	// Setup the kubectl config and context.
	options := k8s.NewKubectlOptions("", "", "default")

	setupPodIdentity(t, options)
	ensureApplicationPodIsDeleted(t, options, podName, podPath)

	k8s.RunKubectlAndGetOutputE(t, options, "delete", "daemonset", "nmi")
	ensureNMIPodsAreDeleted(t, options)
	k8s.KubectlApply(t, options, podPath)
	defer k8s.KubectlDelete(t, options, podPath)
	// If we don't expect success the pod will start normally
	if !expectSuccess {
		for !checkIfPodReady(t, options, podName) {
			time.Sleep(5 * time.Second)
		}
		// in this case we expect the pod to keep running
		assertPodKeepRunningAndDontRestart(t, options, podName)
	} else {
		time.Sleep(10 * time.Second)
		// Assert that the pod is getting restarted. While the pod is still alive the pod should be tagged as not ready.
		if isInitContainer {
			checkPodStayNotReady(t, options, podName)
		} else {
			checkPodRestartAndStayNotReady(t, options, podName)
		}
	}
}
