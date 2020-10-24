package main

import "testing"

const (
	nmiHealthCheckInProbesPath        = "./fixtures/nmiHealthCheckInProbes.yaml"
	nmiHealthCheckInCodePath          = "./fixtures/nmiHealthCheckInCode.yaml"
	nmiHealthCheckInInitContainerPath = "./fixtures/nmiHealthCheckInInitContainer.yaml"
)

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI probes health check in container init
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, there is no way an init container could stop it.
func TestNmiHealthCheckInInitPodShouldStopIfNMIFails(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, nmiHealthCheckInInitContainerPath, false)
}

// If the NMI is missing, the probes should prevent the pod to start.
func TestNmiHealthCheckInInitPodShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, nmiHealthCheckInInitContainerPath, true, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI Health check as part of application probes
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, the pods probe should be not ready and be restarted by health prove
func TestNMIHealthCheckPodShouldBecomeNotReadyAndRestartIfNMIFails(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, nmiHealthCheckInProbesPath, true)
}

// If the NMI is not ready, the probes should prevent the pod to start.
func TestNMIHealthCheckPodShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, nmiHealthCheckInProbesPath, false, true)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test with NMI Health check within application code
////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If the NMI fails or stops, the pods probe should prevent the pod from receiving traffic and restart it.
func TestNMIHealthCheckCustomPodShouldStopIfNMIFails(t *testing.T) {
	DetectNMIFailsAndMakePodUnhealthy(t, nmiHealthCheckInCodePath, true)
}

// If the NMI is missing, the probes should prevent the pod to be ready
func TestNMIHealthCheckCustomPodShouldNeverBeReadyIfNMIIsMissing(t *testing.T) {
	DetectNMIIsNotReadyAndEnsurePodIsNotReady(t, nmiHealthCheckInCodePath, false, true)
}
