package main

import "testing"

const (
	getAccessTokeninHealthProbesPath      = "./fixtures/identityHealthCheckInHealthProbes.yaml"
	identityCheckInInitContainerPath      = "./fixtures/identityCheckInInitContainer.yaml"
	azCliIdentityCheckInInitContainerPath = "./fixtures/identityAzBestPracticesCheckInInitContainer.yaml"
	identityHealthCheckInCode             = "./fixtures/identityHealthCheckInCode.yaml"
)

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full pod identity in init container
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fail, an init container can't ensure ongoing checks for that.
func TestGetAccessTokenInInitProbeWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, identityCheckInInitContainerPath, false)
}

// If the NMI is not there the request will go to the IMDS and it will start normally
// If two identities are present on the cluster, the pod won't start (as the IMDS can't resolve the correct identity). If one identity is there it will.
func TestGetAccessTokenInInitProbeWillNeverBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, identityCheckInInitContainerPath, true, true)
}

// With NMI is up a pod without the appropriate Tag won't be mapped with the identity.
func TestGetAccessTokenInInitProbeWithoutTagsWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, identityCheckInInitContainerPath, true, true)
}

// If the NMI is not here to check, the pod won't be able to start if there are 2 identities on the cluster. The health probe will receive a 400.
func TestGetAccessTokenInInitProbeWithoutTagsWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, identityCheckInInitContainerPath, true, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with az login as init container
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fail, an init container can't pick this up.
func TestIdentityAzLoginCheckInInitContainerWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, azCliIdentityCheckInInitContainerPath, false)
}

// If the NMI is not ready the request will go to the IMDS and it will start normally
func TestAzLoginCheckInInitContainerShouldNeverBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, azCliIdentityCheckInInitContainerPath, true, false)
}

// NMI will refuse to forward the authorization request. It will fail
func TestAzLoginCheckInInitContainerWithoutTagsWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, azCliIdentityCheckInInitContainerPath, true, true)
}

// If the NMI is not here to check, the pod won't be able to start if there are 2 identities on the cluster. The health probe will receive a 400.
func TestAzLoginCheckInInitContainerWithoutTagsWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, azCliIdentityCheckInInitContainerPath, true, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full identity check as part of the Health Checks
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, this solution won't find it until token expire, as the NMI got a token from the application.
func TestGetAccessTokenInHealthProbesWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, getAccessTokeninHealthProbesPath, false)
}

// As the NMI is available the pod will initially succeed in getting the identity. When the NMI fails the pod will hit the IMDS where his token is cached.
// Therefore this highlight this method is unreliable as the pod will only start
func TestGetAccessTokenInHealthProbesWillNotBeStoppedIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, getAccessTokeninHealthProbesPath, false, false)
}

// Check will detect that when NMI is up a pod without the appropriate Tag won't be mapped with the identity.
func TestGetAccessTokenInHealthProbesWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, getAccessTokeninHealthProbesPath, true, true)
}

func TestGetAccessTokenInHealthProbesWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, getAccessTokeninHealthProbesPath, true, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full identity check as part of the code
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, there is no way an init container could stop it.
func TestGetTokenInCodeWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, identityHealthCheckInCode, false)
}

// If the NMI is missing, the az cli will have cached the credentials therefore there is no way to stop the already pod.
func TestGetTokenInCodeWillBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, identityHealthCheckInCode, false, false)
}

// Check will detect that when NMI is up a pod without the appropriate Tag won't be mapped with the identity.
func TestGetTokenInCodeWithoutTagsWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, getAccessTokeninHealthProbesPath, true, true)
}

func TestGetTokenInCodeWithoutTagsWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, getAccessTokeninHealthProbesPath, true, true)
}
