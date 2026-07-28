package host

import factoryhostinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"

// Bundle is the runtime wiring produced by Build and referenced from live handles.
type Bundle = factoryhostinternal.Bundle

// Handle is the live hosted runtime handle.
type Handle = factoryhostinternal.Handle

// LifecycleService is the host implementation of the public Factory Runtime
// lifecycle contract.
type LifecycleService = factoryhostinternal.LifecycleService

// SidecarStarter attaches hosted runtime sidecars after readiness.
type SidecarStarter = factoryhostinternal.SidecarStarter

// ReplacementAttempt pauses sidecars during a replacement attempt.
type ReplacementAttempt = factoryhostinternal.ReplacementAttempt

// NewBundle constructs one inert runtime host from direct collaborators.
var NewBundle = factoryhostinternal.NewBundle

// NewLifecycleService constructs the host lifecycle service.
var NewLifecycleService = factoryhostinternal.NewLifecycleService

// Start begins hosted runtime execution for one bundle.
var Start = factoryhostinternal.Start

// WaitForStart waits until the hosted runtime is ready.
var WaitForStart = factoryhostinternal.WaitForStart

// Stop stops a hosted runtime handle.
var Stop = factoryhostinternal.Stop

// FinalizeArtifacts finalizes recording artifacts for one bundle.
var FinalizeArtifacts = factoryhostinternal.FinalizeArtifacts

// CloseBundleSinks closes log and metrics sinks for one bundle.
var CloseBundleSinks = factoryhostinternal.CloseBundleSinks

// ObserveRuntimeMetrics observes runtime metrics for one handle.
var ObserveRuntimeMetrics = factoryhostinternal.ObserveRuntimeMetrics

// RuntimeStopOutcome derives lifecycle-stop labels from the terminal engine snapshot.
var RuntimeStopOutcome = factoryhostinternal.RuntimeStopOutcome

// StopSidecars cancels and waits for sidecar goroutines attached to handle.
var StopSidecars = factoryhostinternal.StopSidecars

// StartReplacement starts a replacement runtime and optionally attaches sidecars.
var StartReplacement = factoryhostinternal.StartReplacement

// ReplacementFactoryChangePayload extracts factory-change payload from events.
var ReplacementFactoryChangePayload = factoryhostinternal.ReplacementFactoryChangePayload

// PublishFactoryChange publishes a factory change event through one handle.
var PublishFactoryChange = factoryhostinternal.PublishFactoryChange

// SubmitWorkRequest submits work through one hosted bundle.
var SubmitWorkRequest = factoryhostinternal.SubmitWorkRequest

// MoveWork moves work through one hosted bundle.
var MoveWork = factoryhostinternal.MoveWork

// SubscribeFactoryEvents subscribes to factory events for one bundle.
var SubscribeFactoryEvents = factoryhostinternal.SubscribeFactoryEvents

// SubscribeFactoryEventsForSession subscribes to factory events for one session.
var SubscribeFactoryEventsForSession = factoryhostinternal.SubscribeFactoryEventsForSession

// GetEngineStateSnapshot returns the current engine state snapshot.
var GetEngineStateSnapshot = factoryhostinternal.GetEngineStateSnapshot

// WaitToComplete waits for runtime completion on one bundle.
var WaitToComplete = factoryhostinternal.WaitToComplete

// ScriptMetricTimedOut reports whether a script result timed out.
var ScriptMetricTimedOut = factoryhostinternal.ScriptMetricTimedOut

// ScriptMetricFailureReason extracts a script failure reason for metrics.
var ScriptMetricFailureReason = factoryhostinternal.ScriptMetricFailureReason

// ScriptMetricDurationMilliseconds extracts script duration for metrics.
var ScriptMetricDurationMilliseconds = factoryhostinternal.ScriptMetricDurationMilliseconds
