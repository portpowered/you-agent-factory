import type { EmulatorScenario } from "./generated/scenario.js";

export type EmulatorScenarioDiagnosticCode =
  | "INVALID_SCENARIO_SHAPE"
  | "UNSUPPORTED_SCENARIO_VERSION"
  | "INVALID_FACTORY_DEFINITION"
  | "UNSUPPORTED_FACTORY_CAPABILITY";

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
