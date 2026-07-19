import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import type { FactoryEventSink } from "./event-sink.js";
import type {
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

export type FactoryEmulatorCommand = "start";

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
  readonly phase: "sink-write";
  readonly eventCount: number;
}

export interface FactoryEmulatorSessionError {
  readonly code: "sink_write_rejected";
  readonly operation: "write";
  readonly command: FactoryEmulatorCommand;
  readonly message: string;
}

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

export interface FactoryEmulatorSession {
  start(): Promise<FactoryEmulatorStartReceipt>;
  state(): FactoryEmulatorSessionState;
  status(): FactoryEmulatorSessionStatus;
}
