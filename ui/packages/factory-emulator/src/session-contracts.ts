import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import type { FactoryEventSink } from "./event-sink.js";
import type {
  FactoryEmulatorInitialSubmission,
  FactoryEmulatorScenario,
  FactoryEmulatorScenarioIssue,
} from "./scenario-contracts.js";

export const DEFAULT_FACTORY_EMULATOR_LIMITS = Object.freeze({
  maxCompletedDispatches: 1_000,
  maxEvents: 10_000,
  maxVirtualElapsedMs: 3_600_000,
  maxZeroDurationBatches: 1_000,
  maxSynchronousBatches: 100,
  maxSynchronousWorkItems: 1_000,
});

export const FACTORY_EMULATOR_LIMIT_HARD_CAPS = Object.freeze({
  maxCompletedDispatches: 100_000,
  maxEvents: 1_000_000,
  maxVirtualElapsedMs: 86_400_000,
  maxZeroDurationBatches: 10_000,
  maxSynchronousBatches: 10_000,
  maxSynchronousWorkItems: 100_000,
});

export interface FactoryEmulatorLimits {
  readonly maxCompletedDispatches?: number;
  readonly maxEvents?: number;
  readonly maxVirtualElapsedMs?: number;
  readonly maxZeroDurationBatches?: number;
  readonly maxSynchronousBatches?: number;
  readonly maxSynchronousWorkItems?: number;
}

export type ResolvedFactoryEmulatorLimits = Readonly<
  Required<FactoryEmulatorLimits>
>;

export type FactoryEmulatorConfigurationDiagnostic =
  | {
      readonly code:
        | "incompatible_factory"
        | "invalid_factory"
        | "invalid_limit"
        | "invalid_sink"
        | "invalid_yield_control";
      readonly path: readonly (string | number)[];
      readonly message: string;
    }
  | FactoryEmulatorScenarioIssue;

export class FactoryEmulatorConfigurationError extends Error {
  readonly code = "invalid_configuration" as const;
  readonly diagnostics: readonly FactoryEmulatorConfigurationDiagnostic[];

  constructor(diagnostics: readonly FactoryEmulatorConfigurationDiagnostic[]) {
    super("Factory emulator session configuration is invalid.");
    this.name = "FactoryEmulatorConfigurationError";
    this.diagnostics = JSON.parse(
      JSON.stringify(diagnostics),
    ) as readonly FactoryEmulatorConfigurationDiagnostic[];
  }
}

export type FactoryEmulatorCommand =
  | "start"
  | "submit"
  | "advanceBy"
  | "advanceToNext"
  | "close"
  | "reset";

export class FactoryEmulatorLifecycleError extends Error {
  readonly code = "invalid_lifecycle" as const;
  readonly command: FactoryEmulatorCommand;
  readonly lifecycle: FactoryEmulatorSessionState["lifecycle"];

  constructor(
    command: FactoryEmulatorCommand,
    lifecycle: FactoryEmulatorSessionState["lifecycle"],
  ) {
    super(`Cannot ${command} an emulator session in ${lifecycle} lifecycle.`);
    this.name = "FactoryEmulatorLifecycleError";
    this.command = command;
    this.lifecycle = lifecycle;
  }
}

export class FactoryEmulatorPendingCommandError extends Error {
  readonly code = "pending_transaction" as const;
  readonly attemptedCommand: FactoryEmulatorCommand;
  readonly pendingCommand: FactoryEmulatorCommand;

  constructor(
    attemptedCommand: FactoryEmulatorCommand,
    pendingCommand: FactoryEmulatorCommand,
  ) {
    super(
      `Cannot ${attemptedCommand} while ${pendingCommand} is awaiting the event sink.`,
    );
    this.name = "FactoryEmulatorPendingCommandError";
    this.attemptedCommand = attemptedCommand;
    this.pendingCommand = pendingCommand;
  }
}

export class FactoryEmulatorDurationError extends Error {
  readonly code = "invalid_duration" as const;
  readonly durationMs: number;

  constructor(durationMs: number) {
    super("durationMs must be a non-negative safe integer.");
    this.name = "FactoryEmulatorDurationError";
    this.durationMs = durationMs;
  }
}

export class FactoryEmulatorSubmissionError extends Error {
  readonly code = "invalid_submission" as const;
  readonly diagnostics: readonly FactoryEmulatorScenarioIssue[];

  constructor(diagnostics: readonly FactoryEmulatorScenarioIssue[]) {
    super("Factory emulator Work submission is invalid.");
    this.name = "FactoryEmulatorSubmissionError";
    this.diagnostics = JSON.parse(
      JSON.stringify(diagnostics),
    ) as readonly FactoryEmulatorScenarioIssue[];
  }
}

export interface FactoryEmulatorSessionOptions {
  readonly factory: FactoryDefinition;
  readonly scenario: FactoryEmulatorScenario;
  readonly sink: FactoryEventSink;
  readonly limits?: FactoryEmulatorLimits;
  /** Host-owned cooperative boundary. The kernel never schedules browser timers. */
  readonly yieldControl?: () => void | PromiseLike<void>;
}

export interface FactoryEmulatorSessionCounters {
  readonly commands: number;
  readonly events: number;
  readonly completedDispatches: number;
}

export interface FactoryEmulatorSessionWork {
  readonly submissionId: string;
  readonly requestId: string;
  readonly traceId: string;
  readonly workId: string;
  readonly workType: string;
  readonly state: string;
  readonly input?: string;
  readonly parent?: string;
  readonly phase: "ready" | "waiting" | "active" | "completed";
  readonly dispatch?: {
    readonly dispatchId: string;
    readonly completionId: string;
    readonly transitionId: string;
    readonly workstation: string;
    readonly worker: string;
    readonly startedElapsedMs: number;
    readonly dueElapsedMs: number;
    readonly outcome: import("./scenario-contracts.js").FactoryEmulatorOutcome;
  };
}

export type FactoryEmulatorSessionState =
  | {
      readonly lifecycle: "pre-start";
      readonly virtualElapsedMs: 0;
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

export interface FactoryEmulatorBudgetUsage {
  readonly completedDispatches: {
    readonly used: number;
    readonly limit: number;
  };
  readonly events: { readonly used: number; readonly limit: number };
  readonly virtualElapsedMs: { readonly used: number; readonly limit: number };
}

export interface FactoryEmulatorPendingTransactionStatus {
  readonly command: FactoryEmulatorCommand;
  readonly phase: "sink-write" | "sink-close";
  readonly eventCount: number;
}

export type FactoryEmulatorSessionError =
  | {
      readonly code: "sink_write_rejected";
      readonly operation: "write";
      readonly command: FactoryEmulatorCommand;
      readonly message: string;
    }
  | {
      readonly code: "sink_close_rejected";
      readonly operation: "close";
      readonly command: FactoryEmulatorCommand;
      readonly message: string;
    };

export interface FactoryEmulatorSessionStatus {
  readonly phase: "error" | "closed" | "active" | "ready" | "waiting" | "idle";
  readonly reason: string;
  readonly virtualTime?: string;
  readonly virtualElapsedMs: number;
  readonly budgetUsage: FactoryEmulatorBudgetUsage;
  readonly pendingTransaction?: FactoryEmulatorPendingTransactionStatus;
  readonly error?: FactoryEmulatorSessionError;
}

export interface FactoryEmulatorStartReceipt {
  readonly status: "started";
  readonly batches: readonly (readonly FactoryEvent[])[];
  readonly state: Extract<
    FactoryEmulatorSessionState,
    { lifecycle: "started" }
  >;
}

export interface FactoryEmulatorSubmitReceipt {
  readonly status: "submitted";
  readonly batch: readonly FactoryEvent[];
  readonly state: Extract<
    FactoryEmulatorSessionState,
    { lifecycle: "started" }
  >;
}

export interface FactoryEmulatorAdvanceReceipt {
  readonly status: "advanced" | "idle";
  readonly command: "advanceBy" | "advanceToNext";
  readonly fromVirtualTime: string;
  readonly virtualTime: string;
  readonly virtualElapsedMs: number;
  readonly batches: readonly (readonly FactoryEvent[])[];
  readonly state: Extract<
    FactoryEmulatorSessionState,
    { lifecycle: "started" }
  >;
}

export interface FactoryEmulatorCloseReceipt {
  readonly status: "closed";
  readonly batch: readonly FactoryEvent[];
  readonly state: Extract<FactoryEmulatorSessionState, { lifecycle: "closed" }>;
}

export interface FactoryEmulatorResetReceipt {
  readonly status: "reset";
  readonly state: Extract<
    FactoryEmulatorSessionState,
    { lifecycle: "pre-start" }
  >;
}

export interface FactoryEmulatorSession {
  start(): Promise<FactoryEmulatorStartReceipt>;
  submit(
    submissionOrBatch:
      | FactoryEmulatorInitialSubmission
      | readonly FactoryEmulatorInitialSubmission[],
  ): Promise<FactoryEmulatorSubmitReceipt>;
  advanceBy(durationMs: number): Promise<FactoryEmulatorAdvanceReceipt>;
  advanceToNext(): Promise<FactoryEmulatorAdvanceReceipt>;
  close(): Promise<FactoryEmulatorCloseReceipt>;
  reset(): FactoryEmulatorResetReceipt;
  state(): FactoryEmulatorSessionState;
  status(): FactoryEmulatorSessionStatus;
}
