import type { EmulatorScenario } from "./generated/scenario.js";
import type { EmulatorFactoryDefinition } from "./parser.js";

export interface EmulatorScenarioExample {
  readonly name: "minimal" | "multi-rule-lineage";
  readonly factory: EmulatorFactoryDefinition;
  readonly scenario: EmulatorScenario;
}

/** Published, parser-validated documents for consumers starting an emulator host. */
export declare const emulatorScenarioExamples: readonly EmulatorScenarioExample[];
