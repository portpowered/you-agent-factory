package factorysessionexecution

import factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

func (s *JavaScriptRuntimeService) childExecutorHooks(mode, sessionID string) factory.JavaScriptRuntimeHooks {
	hooks := factory.JavaScriptRuntimeHooks{
		OnRecord: func(record factory.JavaScriptRuntimeRecord) {
			s.applyRunningRuntimeRecord(sessionID, record)
		},
	}
	if mode != ChildExecutorModeLive {
		return hooks
	}
	hooks.NewChildExecutor = func(childSessionID string, records factory.JavaScriptChildRecordSink, policy factory.JavaScriptPolicy) factory.JavaScriptChildExecutor {
		// Which executor serves a session is decided by which composition built
		// this service, not by anything on the request. A runtime-backed session
		// invokes its children as Workers through the already-composed Execute
		// capability; the standalone `you run script.js` composition builds no
		// runtime and reaches the same Execute capability directly.
		if binding := s.workerExecutionBinding(); binding != nil {
			executor := newChildWorkerExecutor(
				childSessionID,
				binding.execute,
				records,
				s.childValues,
				s.observeWorkerDispatch,
				s.projectRoot,
				policy.MaxRetries,
			)
			executor.resourceLeaseAcquirer = binding.resourceLeaseAcquirer
			executor.runtimeID = binding.runtimeID
			executor.generationID = binding.generationID
			executor.providerOverride = binding.providerOverride
			executor.mockWorkers = binding.mockWorkers.Clone()
			executor.commandRunnerOverride = binding.commandRunnerOverride
			executor.publish = binding.publish
			return executor
		}
		if execution := s.directWorkerExecution(); execution != nil {
			return newDirectChildExecutor(
				childSessionID,
				execution,
				records,
				s.childValues,
				s.projectRoot,
			)
		}
		// Compatibility is retained for in-package callers that have not yet
		// moved to the standalone Workers binding. It is not part of the Wire
		// production path and is the P6-C retirement survivor.
		if s.directChildInvocation == nil {
			return newDirectChildExecutor(
				childSessionID,
				nil,
				records,
				s.childValues,
				s.projectRoot,
			)
		}
		return newDirectChildExecutor(
			childSessionID,
			legacyDirectChildExecution{invocation: s.directChildInvocation},
			records,
			s.childValues,
			s.projectRoot,
		)
	}
	return hooks
}
