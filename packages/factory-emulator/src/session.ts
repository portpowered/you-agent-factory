import type { FactoryEventBatch, FactoryEventSink } from "./contracts.js";
import type {
  EmulatorInitialSubmission,
  EmulatorOutcome,
  EmulatorScenario,
} from "./generated/scenario.js";
import type {
  EmulatorFactoryDefinition,
  EmulatorScenarioDiagnostic,
} from "./parser.js";

export type FactoryEmulatorConfigurationDiagnostic =
  | EmulatorScenarioDiagnostic
  | {
      readonly code: "INVALID_LIMIT_CONFIGURATION";
      readonly path: string;
      readonly message: string;
      readonly expectation: string;
    };

export interface FactoryEmulatorDataError {
  readonly name: string;
  readonly message: string;
  readonly code: string;
}

export interface FactoryEmulatorConfigurationError extends FactoryEmulatorDataError {
  readonly name: "FactoryEmulatorConfigurationError";
  readonly code: "INVALID_CONFIGURATION";
  readonly diagnostics: readonly FactoryEmulatorConfigurationDiagnostic[];
}
export declare const FactoryEmulatorConfigurationError: {
  new (diagnostics: readonly FactoryEmulatorConfigurationDiagnostic[]): FactoryEmulatorConfigurationError;
};

export interface FactoryEmulatorLifecycleError extends FactoryEmulatorDataError {
  readonly name: "FactoryEmulatorLifecycleError";
  readonly code: "INVALID_LIFECYCLE";
  readonly command: "start" | "submit" | "advanceBy" | "advanceToNext" | "close" | "reset";
  readonly phase:
    | FactoryEmulatorSessionState["lifecycle"]
    | "starting"
    | "submitting"
    | "advancing"
    | "closing";
}
export declare const FactoryEmulatorLifecycleError: {
  new (
    command: FactoryEmulatorLifecycleError["command"],
    phase: FactoryEmulatorLifecycleError["phase"],
  ): FactoryEmulatorLifecycleError;
};

export interface FactoryEmulatorDurationError extends FactoryEmulatorDataError {
  readonly name: "FactoryEmulatorDurationError";
  readonly code: "INVALID_DURATION";
  readonly durationMs: number;
}
export declare const FactoryEmulatorDurationError: {
  new (durationMs: number): FactoryEmulatorDurationError;
};

export interface FactoryEmulatorSubmissionError extends FactoryEmulatorDataError {
  readonly name: "FactoryEmulatorSubmissionError";
  readonly code: "INVALID_SUBMISSION";
  readonly diagnostics: readonly EmulatorScenarioDiagnostic[];
}
export declare const FactoryEmulatorSubmissionError: {
  new (diagnostics: readonly EmulatorScenarioDiagnostic[]): FactoryEmulatorSubmissionError;
};

export type FactoryEmulatorExecutionDiagnostic =
  | {
      readonly kind: "budget-exceeded";
      readonly limit: "completedDispatches" | "events" | "virtualElapsedMs";
      readonly configured: number;
      readonly observed: number;
      readonly virtualTime: string;
      readonly virtualElapsedMs: number;
    }
  | {
      readonly kind: "zero-duration-cycle";
      readonly limit: "zeroDurationBatches";
      readonly configured: number;
      readonly observed: number;
      readonly virtualTime: string;
      readonly virtualElapsedMs: number;
    };

export interface FactoryEmulatorExecutionPausedError extends FactoryEmulatorDataError {
  readonly name: "FactoryEmulatorExecutionPausedError";
  readonly code: "EXECUTION_PAUSED";
  readonly diagnostic: FactoryEmulatorExecutionDiagnostic;
}
export declare const FactoryEmulatorExecutionPausedError: {
  new (diagnostic: FactoryEmulatorExecutionDiagnostic): FactoryEmulatorExecutionPausedError;
};

export interface FactoryEmulatorLimits {
  readonly maxCompletedDispatches?: number;
  readonly maxEvents?: number;
  readonly maxVirtualElapsedMs?: number;
  readonly maxZeroDurationBatches?: number;
  readonly maxSynchronousBatches?: number;
}

export declare const DEFAULT_FACTORY_EMULATOR_LIMITS: Readonly<Required<FactoryEmulatorLimits>>;
export declare const FACTORY_EMULATOR_LIMIT_HARD_CAPS: Readonly<Required<FactoryEmulatorLimits>>;

export interface FactoryEmulatorPendingCommandError extends FactoryEmulatorDataError {
  readonly name: "FactoryEmulatorPendingCommandError";
  readonly code: "PENDING_TRANSACTION";
  readonly attemptedCommand:
    | "start"
    | "submit"
    | "advanceBy"
    | "advanceToNext"
    | "close";
  readonly pendingCommand:
    | "start"
    | "submit"
    | "advanceBy"
    | "advanceToNext"
    | "close";
}
export declare const FactoryEmulatorPendingCommandError: {
  new (
    attemptedCommand: FactoryEmulatorPendingCommandError["attemptedCommand"],
    pendingCommand: FactoryEmulatorPendingCommandError["pendingCommand"],
  ): FactoryEmulatorPendingCommandError;
};

export type FactoryEmulatorCommandError =
  | FactoryEmulatorConfigurationError
  | FactoryEmulatorDurationError
  | FactoryEmulatorExecutionPausedError
  | FactoryEmulatorLifecycleError
  | FactoryEmulatorPendingCommandError
  | FactoryEmulatorSubmissionError;

export interface FactoryEmulatorSessionOptions {
  readonly factory: EmulatorFactoryDefinition;
  readonly scenario: EmulatorScenario;
  readonly sink: FactoryEventSink;
  readonly limits?: FactoryEmulatorLimits;
}

export interface FactoryEmulatorSessionWork {
  readonly submissionId: string;
  readonly requestId: string;
  readonly traceId: string;
  readonly workId: string;
  readonly workType: string;
  readonly phase: "active" | "ready" | "waiting" | "completed";
  readonly input?: Readonly<Record<string, unknown>>;
  readonly output?: Readonly<Record<string, unknown>>;
  readonly rejectionReason?: string;
  readonly dispatch?: {
    readonly dispatchId: string;
    readonly completionId: string;
    readonly transitionId: string;
    readonly startedElapsedMs: number;
    readonly dueElapsedMs: number;
    readonly outcome: EmulatorOutcome;
  };
}

export interface FactoryEmulatorSessionCounters {
  readonly commands: number;
  readonly events: number;
  readonly requests: number;
  readonly works: number;
  readonly dispatches: number;
  readonly completions: number;
}

export type FactoryEmulatorSessionState =
  | {
      readonly lifecycle: "pre-start";
      readonly virtualElapsedMs: 0;
      readonly works: readonly [];
      readonly ruleCursors: Readonly<Record<string, number>>;
      readonly counters: FactoryEmulatorSessionCounters;
    }
  | {
      readonly lifecycle: "started";
      readonly sessionId: string;
      readonly virtualTime: string;
      readonly virtualElapsedMs: number;
      readonly works: readonly FactoryEmulatorSessionWork[];
      readonly ruleCursors: Readonly<Record<string, number>>;
      readonly counters: FactoryEmulatorSessionCounters;
    }
  | {
      readonly lifecycle: "closed";
      readonly sessionId: string;
      readonly virtualTime: string;
      readonly virtualElapsedMs: number;
      readonly works: readonly FactoryEmulatorSessionWork[];
      readonly ruleCursors: Readonly<Record<string, number>>;
      readonly counters: FactoryEmulatorSessionCounters;
    };

export interface FactoryEmulatorStartReceipt {
  readonly status: "started";
  readonly batches: readonly FactoryEventBatch[];
  readonly state: Extract<FactoryEmulatorSessionState, { lifecycle: "started" }>;
}

export interface FactoryEmulatorResetReceipt {
  readonly status: "reset";
  readonly state: Extract<FactoryEmulatorSessionState, { lifecycle: "pre-start" }>;
}

export interface FactoryEmulatorSubmitReceipt {
  readonly status: "submitted";
  readonly batch: FactoryEventBatch;
  readonly state: Extract<FactoryEmulatorSessionState, { lifecycle: "started" }>;
}

export interface FactoryEmulatorSessionAdvanceReceipt {
  readonly status: "advanced" | "idle";
  readonly command: "advanceBy" | "advanceToNext";
  readonly fromVirtualTime: string;
  readonly virtualTime: string;
  readonly virtualElapsedMs: number;
  readonly batches: readonly FactoryEventBatch[];
  readonly state: Extract<FactoryEmulatorSessionState, { lifecycle: "started" }>;
}

export interface FactoryEmulatorSessionCloseReceipt {
  readonly status: "closed";
  readonly batch: FactoryEventBatch;
  readonly state: Extract<FactoryEmulatorSessionState, { lifecycle: "closed" }>;
}

export interface FactoryEmulatorBudgetUsage {
  readonly completedDispatches: {
    readonly used: number;
    readonly limit: number;
  };
  readonly events: {
    readonly used: number;
    readonly limit: number;
  };
  readonly virtualElapsedMs: {
    readonly used: number;
    readonly limit: number;
  };
}

export type FactoryEmulatorSessionError =
  | {
      readonly code: "EXECUTION_PAUSED";
      readonly diagnostic: FactoryEmulatorExecutionDiagnostic;
    }
  | {
      readonly code: "SINK_WRITE_REJECTED" | "SINK_CLOSE_REJECTED";
      readonly operation: "write" | "close";
      readonly command: "start" | "submit" | "advanceBy" | "advanceToNext" | "close";
      readonly message: string;
    };

export interface FactoryEmulatorPendingTransactionStatus {
  readonly command: "start" | "submit" | "advanceBy" | "advanceToNext" | "close";
  readonly phase: "sink-write" | "sink-close";
  readonly eventCount: number;
}

export interface FactoryEmulatorSessionStatus {
  readonly phase:
    | "error"
    | "closed"
    | "active"
    | "ready"
    | "waiting"
    | "idle";
  readonly reason: string;
  readonly virtualTime?: string;
  readonly virtualElapsedMs: number;
  readonly budgetUsage: FactoryEmulatorBudgetUsage;
  readonly pendingTransaction?: FactoryEmulatorPendingTransactionStatus;
  readonly error?: FactoryEmulatorSessionError;
  readonly diagnostic?: FactoryEmulatorExecutionDiagnostic;
}

export interface FactoryEmulatorSession {
  start(): Promise<FactoryEmulatorStartReceipt>;
  submit(
    submissionOrBatch: EmulatorInitialSubmission | readonly EmulatorInitialSubmission[],
  ): Promise<FactoryEmulatorSubmitReceipt>;
  advanceBy(durationMs: number): Promise<FactoryEmulatorSessionAdvanceReceipt>;
  advanceToNext(): Promise<FactoryEmulatorSessionAdvanceReceipt>;
  close(): Promise<FactoryEmulatorSessionCloseReceipt>;
  reset(): FactoryEmulatorResetReceipt;
  pending(): FactoryEventBatch | undefined;
  state(): FactoryEmulatorSessionState;
  status(): FactoryEmulatorSessionStatus;
}

/** Creates one validated, caller-owned deterministic Factory emulator session. */
export declare function createFactoryEmulatorSession(
  options: FactoryEmulatorSessionOptions,
): FactoryEmulatorSession;
