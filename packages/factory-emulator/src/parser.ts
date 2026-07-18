import type { EmulatorScenario } from "./generated/scenario.js";

export type EmulatorScenarioDiagnosticCode =
  | "INVALID_SCENARIO_SHAPE"
  | "UNSUPPORTED_SCENARIO_VERSION"
  | "INVALID_FACTORY_DEFINITION"
  | "UNSUPPORTED_FACTORY_CAPABILITY"
  | "DUPLICATE_SCENARIO_IDENTIFIER"
  | "UNKNOWN_FACTORY_WORK_TYPE"
  | "UNKNOWN_INITIAL_SUBMISSION"
  | "MISSING_LINEAGE_CURSOR_TARGET"
  | "FORWARD_LINEAGE_CURSOR"
  | "CYCLIC_LINEAGE_CURSOR"
  | "INCOMPATIBLE_LINEAGE_CURSOR"
  | "SHADOWED_RULE";

export interface EmulatorScenarioDiagnostic {
  code: EmulatorScenarioDiagnosticCode;
  path: string;
  message: string;
  expectation: string;
}

/** The executable Factory subset accepted by the v1 browser emulator. */
export interface EmulatorFactoryDefinition {
  orchestrator?: { kind?: string };
  resources?: unknown[];
  guards?: unknown[];
  workTypes?: Array<{ name?: string }>;
  workstations?: Array<{ behavior?: string; cron?: unknown }>;
}

export type EmulatorScenarioParseResult =
  | {
      success: true;
      scenario: EmulatorScenario;
      factory: EmulatorFactoryDefinition;
    }
  | { success: false; diagnostics: readonly EmulatorScenarioDiagnostic[] };

/**
 * Validates scenario structure and Factory execution support without creating
 * Factory events or starting emulator activity.
 */
export declare function parseEmulatorScenario(
  scenario: unknown,
  factory: unknown,
): EmulatorScenarioParseResult;
