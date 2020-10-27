package main

import "testing"

const (
	getAccessTokeninHealthProbesPath      = "./fixtures/identityHealthCheckInHealthProbes.yaml"
	identityCheckInInitContainerPath      = "./fixtures/identityCheckInInitContainer.yaml"
	azCliIdentityCheckInInitContainerPath = "./fixtures/identityAzBestPracticesCheckInInitContainer.yaml"
	identityHealthCheckInCode             = "./fixtures/identityHealthCheckInCode.yaml"
)

/// it is advised to run tests in sequence and not at once, due to number of moving pieces they could be flaky.

/// SET TO FALSE IF CLUSTER HAS ONLY 1 USER ASSIGNED IDENTITIES OR A SYSTEM ASSIGNED IDENTITY IS ON THE CLUSTER
/// SET TO TRUE IF CLUSTER HAS NO SYSTEM ASSIGNED IDENTITY AND >1 USER ASSIGNED IDENTITY
/// VALUE TRUE IS HIGHLY FLAKY AS IT DEPENDS ON THE IMDS TOKEN CACHE. THIS ILLUSTRATE THE UNRELIABILITY OF THOSE METHODS.
// if you set value to true and in order to reduce test flakiness it is recommend to manually add two user-assigned identites (different from the one used by pod identity) to reduce flakiness of these tests.
const isMultiUserAssignedIdentityCluster = false

// SET TO TRUE IF YOU HAVE A SYSTEM ASSIGNED IDENTITY FALSE OTHERWISE.
const isSystemAssignedIdentity = false

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
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, identityCheckInInitContainerPath, true, isMultiUserAssignedIdentityCluster)
}

// With NMI is up a pod without the appropriate Tag won't be mapped with the identity.
func TestGetAccessTokenInInitProbeWithoutTagsWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, identityCheckInInitContainerPath, true, true)
}

// Should Not succeed If the NMI is not here to check, the pod won't be able to start if there are 2 identities on the cluster. The health probe will receive a 400.
func TestGetAccessTokenInInitProbeWithoutTagsWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, identityCheckInInitContainerPath, true, isMultiUserAssignedIdentityCluster)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with az login as init container
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fail at runtime, an init container can't pick this up.
func TestIdentityAzLoginCheckInInitContainerWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, azCliIdentityCheckInInitContainerPath, false)
}

// If the NMI is not ready the request will go to the IMDS and it will start normally
// In case of System assigned identity, it will detect that, but not in case of a single user assigned identity
func TestAzLoginCheckInInitContainerShouldNeverBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, azCliIdentityCheckInInitContainerPath, true, isSystemAssignedIdentity || isMultiUserAssignedIdentityCluster)
}

// NMI will refuse to forward the authorization request. It will fail
func TestAzLoginCheckInInitContainerWithoutTagsWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, azCliIdentityCheckInInitContainerPath, true, true)
}

// If the NMI is not here to check, the pod won't be able to start if there are 2 identities on the cluster. The health probe will receive a 400.
// If only one id is there the az cli will login.
// a system  a
func TestAzLoginCheckInInitContainerWithoutTagsWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, azCliIdentityCheckInInitContainerPath, true, isSystemAssignedIdentity || isMultiUserAssignedIdentityCluster)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full identity check as part of the Health Checks
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, this solution won't find it until token expire, as the NMI got a token from the application.
func TestGetAccessTokenInHealthProbesWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, getAccessTokeninHealthProbesPath, isMultiUserAssignedIdentityCluster)
}

// As the NMI is available the pod will initially succeed in getting the identity. When the NMI fails the pod will hit the IMDS where his token is cached.
// Therefore this highlight this method is unreliable as the pod will only start
func TestGetAccessTokenInHealthProbesWillNotBeStoppedIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, getAccessTokeninHealthProbesPath, false, isMultiUserAssignedIdentityCluster)
}

// Check will detect that when NMI is up a pod without the appropriate Tag won't be mapped with the identity.
func TestGetAccessTokenInHealthProbesWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, getAccessTokeninHealthProbesPath, false, true)
}

func TestGetAccessTokenInHealthProbesWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, getAccessTokeninHealthProbesPath, false, isMultiUserAssignedIdentityCluster)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with full identity check as part of the code
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, there is no way an init container could stop it.
func TestGetTokenInCodeWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, identityHealthCheckInCode, isMultiUserAssignedIdentityCluster)
}

// If the NMI is missing, the az cli will have cached the credentials therefore there is no way to stop the already pod.
func TestGetTokenInCodeWillBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, identityHealthCheckInCode, false, isMultiUserAssignedIdentityCluster)
}

// Check will detect that when NMI is up a pod without the appropriate Tag won't be mapped with the identity.
func TestGetTokenInCodeWithoutTagsWillBeRejectedWhenNMIIsUp(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsUp(t, getAccessTokeninHealthProbesPath, false, true)
}

func TestGetTokenInCodeWithoutTagsWillBeRejectedWhenNMIIsDown(t *testing.T) {
	PodIsRejectedWhenNoLabelWhenNMIIsDown(t, getAccessTokeninHealthProbesPath, false, isMultiUserAssignedIdentityCluster)
}
