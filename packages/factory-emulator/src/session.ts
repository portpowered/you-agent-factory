import type { FactoryEventBatch, FactoryEventSink } from "./contracts.js";
import type { EmulatorScenario } from "./generated/scenario.js";
import type {
  EmulatorFactoryDefinition,
  EmulatorScenarioDiagnostic,
} from "./parser.js";

export declare class FactoryEmulatorConfigurationError extends Error {
  readonly code: "INVALID_CONFIGURATION";
  readonly diagnostics: readonly EmulatorScenarioDiagnostic[];
}

export declare class FactoryEmulatorLifecycleError extends Error {
  readonly code: "INVALID_LIFECYCLE";
  readonly command: "start" | "reset";
  readonly phase: FactoryEmulatorSessionState["lifecycle"] | "starting";
}

export interface FactoryEmulatorSessionOptions {
  readonly factory: EmulatorFactoryDefinition;
  readonly scenario: EmulatorScenario;
  readonly sink: FactoryEventSink;
}

export interface FactoryEmulatorSessionWork {
  readonly submissionId: string;
  readonly requestId: string;
  readonly traceId: string;
  readonly workId: string;
  readonly workType: string;
  readonly input?: Readonly<Record<string, unknown>>;
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
      readonly virtualElapsedMs: 0;
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

export interface FactoryEmulatorSession {
  start(): Promise<FactoryEmulatorStartReceipt>;
  reset(): FactoryEmulatorResetReceipt;
  state(): FactoryEmulatorSessionState;
}

/** Creates one validated, caller-owned deterministic Factory emulator session. */
export declare function createFactoryEmulatorSession(
  options: FactoryEmulatorSessionOptions,
): FactoryEmulatorSession;
