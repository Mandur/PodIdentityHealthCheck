package main

import "testing"

const (
	nmiHealthCheckInProbesPath        = "./fixtures/nmiHealthCheckInProbes.yaml"
	nmiHealthCheckInCodePath          = "./fixtures/nmiHealthCheckInCode.yaml"
	nmiHealthCheckInInitContainerPath = "./fixtures/nmiHealthCheckInInitContainer.yaml"
)

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI probes health check in init container
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, there is no way an init container could stop it.
func TestNmiHealthCheckInInitWillNotStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, nmiHealthCheckInInitContainerPath, false)
}

// If the NMI is missing, the init container will prevent the pod to start.
func TestNmiHealthCheckInInitPodShouldNeverBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, nmiHealthCheckInInitContainerPath, true, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI Health check as part of application probes
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, the pods probe should become not ready and be restarted by health probe
func TestNMIHealthCheckPodWillBecomeNotReadyAndRestartIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, nmiHealthCheckInProbesPath, true)
}

// If the NMI is not ready, the probes should prevent the pod from being ready and restart it.
func TestNMIHealthCheckPodShouldNeverBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, nmiHealthCheckInProbesPath, false, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI Health check within application code
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, the pods probe should prevent the pod from receiving traffic and restart it.
func TestNMIHealthCheckCustomPodShouldStopIfNMIFailsAtRuntime(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, nmiHealthCheckInCodePath, true)
}

// If the NMI is missing, the probes should prevent the pod to be ready and restart it
func TestNMIHealthCheckCustomPodShouldNeverBeReadyIfNMIIsMissingAtStartup(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, nmiHealthCheckInCodePath, false, true)
}
