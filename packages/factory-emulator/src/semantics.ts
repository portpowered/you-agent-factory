import type {
  EmulatorExhaustionBehavior,
  EmulatorOutcome,
  EmulatorRule,
  EmulatorScenario,
  EmulatorUnmatchedBehavior,
} from "./generated/scenario.js";
import type { EmulatorScenarioDiagnostic } from "./parser.js";

export interface EmulatorSubmission {
  id: string;
  workType: string;
}

export type EmulatorScenarioResolution =
  | { kind: "outcome"; rule: EmulatorRule; outcome: EmulatorOutcome }
  | { kind: "unmatched"; behavior: EmulatorUnmatchedBehavior }
  | {
      kind: "exhausted";
      rule: EmulatorRule;
      behavior: Extract<EmulatorExhaustionBehavior, { kind: "reject" }>;
    };

export declare function selectEmulatorRule(
  scenario: EmulatorScenario,
  submission: EmulatorSubmission,
): EmulatorRule | undefined;

export declare function resolveEmulatorScenarioResult(
  scenario: EmulatorScenario,
  submission: EmulatorSubmission,
  invocationIndex: number,
): EmulatorScenarioResolution;

export declare function scenarioSemanticDiagnostics(
  scenario: EmulatorScenario,
  factory: unknown,
): EmulatorScenarioDiagnostic[];
