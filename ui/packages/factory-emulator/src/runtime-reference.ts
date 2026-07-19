import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { inspectFactoryEmulatorCompatibility } from "./compatibility.js";
import { runtimeReferenceFixtures } from "./runtime-reference-fixtures.js";
import {
  type FactoryEmulatorScenario,
  safeParseFactoryEmulatorScenario,
} from "./scenario.js";

export const FACTORY_EMULATOR_RUNTIME_REFERENCE_SCHEMA_VERSION =
  "factory-emulator-runtime-reference/v1" as const;

const factoryEventKinds = new Set<FactoryEvent["type"]>([
  "RUN_REQUEST",
  "INITIAL_STRUCTURE_REQUEST",
  "SESSION_STARTED",
  "WORK_REQUEST",
  "RELATIONSHIP_CHANGE_REQUEST",
  "DISPATCH_REQUEST",
  "DISPATCH_RESPONSE",
  "WORK_STATE_CHANGE",
  "SESSION_COMPLETED",
]);

export interface FactoryEmulatorRuntimeReferenceProvenance {
  readonly source: string;
  readonly kind:
    | "public-behavior"
    | "public-documentation"
    | "public-recording";
}

export interface FactoryEmulatorRuntimeReferenceTick {
  readonly logicalTick: number;
  readonly eventKinds: readonly FactoryEvent["type"][];
  readonly semantics: FactoryEmulatorRuntimeReferenceSemantics;
}

/** Stable, identity- and time-normalized evidence observed after one logical tick. */
export interface FactoryEmulatorRuntimeReferenceSemantics {
  readonly dispatchChoices: readonly string[];
  readonly consumedWork: readonly string[];
  readonly outcomes: readonly string[];
  readonly routes: readonly string[];
  readonly terminalStates: readonly string[];
  readonly replayProjection: readonly string[];
}

export interface FactoryEmulatorRuntimeReference {
  readonly schemaVersion: typeof FACTORY_EMULATOR_RUNTIME_REFERENCE_SCHEMA_VERSION;
  readonly id: string;
  readonly title: string;
  readonly provenance: FactoryEmulatorRuntimeReferenceProvenance;
  readonly factory: FactoryDefinition;
  readonly scenario: FactoryEmulatorScenario;
  readonly ticks: readonly FactoryEmulatorRuntimeReferenceTick[];
  readonly orderedEventKinds: readonly FactoryEvent["type"][];
}

export type FactoryEmulatorRuntimeReferenceIssueCode =
  | "invalid_factory"
  | "invalid_provenance"
  | "invalid_schema_version"
  | "invalid_tick_order"
  | "invalid_value"
  | "missing_required_data"
  | "missing_semantics"
  | "unexpected_event_kind";

export interface FactoryEmulatorRuntimeReferenceIssue {
  readonly code: FactoryEmulatorRuntimeReferenceIssueCode;
  readonly path: readonly (string | number)[];
  readonly message: string;
}

export type SafeParseFactoryEmulatorRuntimeReferenceResult =
  | { readonly success: true; readonly data: FactoryEmulatorRuntimeReference }
  | {
      readonly success: false;
      readonly issues: readonly FactoryEmulatorRuntimeReferenceIssue[];
    };

function issue(
  issues: FactoryEmulatorRuntimeReferenceIssue[],
  code: FactoryEmulatorRuntimeReferenceIssueCode,
  path: readonly (string | number)[],
  message: string,
): void {
  issues.push({ code, path, message });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function requiredString(
  value: unknown,
  path: readonly (string | number)[],
  issues: FactoryEmulatorRuntimeReferenceIssue[],
): value is string {
  if (typeof value === "string" && value.trim().length > 0) return true;
  issue(
    issues,
    "missing_required_data",
    path,
    `Expected ${path.join(".")} to be a non-empty string.`,
  );
  return false;
}

/** Validates frozen, package-local evidence before a conformance comparison starts. */
// biome-ignore lint/complexity/noExcessiveLinesPerFunction: The ordered validation passes deliberately retain every fixture diagnostic in one stable result.
export function safeParseFactoryEmulatorRuntimeReference(
  input: unknown,
): SafeParseFactoryEmulatorRuntimeReferenceResult {
  const issues: FactoryEmulatorRuntimeReferenceIssue[] = [];
  if (!isRecord(input)) {
    issue(issues, "invalid_value", [], "Expected a runtime reference object.");
    return { success: false, issues };
  }
  if (
    input.schemaVersion !== FACTORY_EMULATOR_RUNTIME_REFERENCE_SCHEMA_VERSION
  ) {
    issue(
      issues,
      "invalid_schema_version",
      ["schemaVersion"],
      "Unsupported runtime reference schema version.",
    );
  }
  requiredString(input.id, ["id"], issues);
  requiredString(input.title, ["title"], issues);
  if (!isRecord(input.provenance)) {
    issue(
      issues,
      "missing_required_data",
      ["provenance"],
      "Expected runtime-reference provenance.",
    );
  } else {
    requiredString(input.provenance.source, ["provenance", "source"], issues);
    if (
      !(
        ["public-behavior", "public-documentation", "public-recording"] as const
      ).includes(input.provenance.kind as never)
    ) {
      issue(
        issues,
        "invalid_provenance",
        ["provenance", "kind"],
        "Expected public behavior, documentation, or recording provenance.",
      );
    }
  }
  if (
    !isRecord(input.factory) ||
    !requiredString(input.factory.name, ["factory", "name"], issues)
  ) {
    issue(
      issues,
      "invalid_factory",
      ["factory"],
      "Expected a Factory definition with a name.",
    );
  }
  if (!isRecord(input.scenario)) {
    issue(
      issues,
      "missing_required_data",
      ["scenario"],
      "Expected a complete emulator scenario.",
    );
  } else if (isRecord(input.factory)) {
    const parsed = safeParseFactoryEmulatorScenario(
      input.scenario,
      input.factory as FactoryDefinition,
    );
    if (!parsed.success) {
      issue(
        issues,
        "invalid_value",
        ["scenario"],
        `Scenario is invalid: ${parsed.issues[0]?.message ?? "unknown error"}`,
      );
    }
  }
  const ticks = Array.isArray(input.ticks) ? input.ticks : [];
  if (ticks.length === 0) {
    issue(
      issues,
      "missing_required_data",
      ["ticks"],
      "Expected at least one logical-tick reference.",
    );
  }
  const eventKinds: FactoryEvent["type"][] = [];
  for (const [index, tick] of ticks.entries()) {
    const logicalTick = isRecord(tick) ? tick.logicalTick : undefined;
    if (
      typeof logicalTick !== "number" ||
      !Number.isSafeInteger(logicalTick) ||
      logicalTick < 0
    ) {
      issue(
        issues,
        "invalid_tick_order",
        ["ticks", index, "logicalTick"],
        "Expected a non-negative integer logical tick.",
      );
    } else if (logicalTick !== index) {
      issue(
        issues,
        "invalid_tick_order",
        ["ticks", index, "logicalTick"],
        "Logical ticks must be contiguous and begin at zero.",
      );
    }
    if (!isRecord(tick) || !Array.isArray(tick.eventKinds)) {
      issue(
        issues,
        "missing_required_data",
        ["ticks", index, "eventKinds"],
        "Expected the ordered event kinds for this tick.",
      );
      continue;
    }
    if (!isRecord(tick.semantics)) {
      issue(
        issues,
        "missing_semantics",
        ["ticks", index, "semantics"],
        "Expected normalized semantic evidence for this logical tick.",
      );
    } else {
      for (const surface of [
        "dispatchChoices",
        "consumedWork",
        "outcomes",
        "routes",
        "terminalStates",
        "replayProjection",
      ] as const) {
        if (!Array.isArray(tick.semantics[surface])) {
          issue(
            issues,
            "missing_semantics",
            ["ticks", index, "semantics", surface],
            `Expected ${surface} semantic evidence.`,
          );
        }
      }
    }
    for (const [eventIndex, eventKind] of tick.eventKinds.entries()) {
      if (!factoryEventKinds.has(eventKind as FactoryEvent["type"])) {
        issue(
          issues,
          "unexpected_event_kind",
          ["ticks", index, "eventKinds", eventIndex],
          `Unexpected Factory event kind ${String(eventKind)}.`,
        );
      } else eventKinds.push(eventKind);
    }
  }
  if (!Array.isArray(input.orderedEventKinds)) {
    issue(
      issues,
      "missing_required_data",
      ["orderedEventKinds"],
      "Expected the complete ordered event-kind sequence.",
    );
  } else if (
    JSON.stringify(input.orderedEventKinds) !== JSON.stringify(eventKinds)
  ) {
    issue(
      issues,
      "invalid_value",
      ["orderedEventKinds"],
      "Ordered event kinds must exactly concatenate the logical-tick event kinds.",
    );
  }
  if (issues.length > 0) return { success: false, issues };
  const reference = input as unknown as FactoryEmulatorRuntimeReference;
  const compatibility = inspectFactoryEmulatorCompatibility(reference.factory);
  if (!compatibility.supported) {
    return {
      success: false,
      issues: [
        {
          code: "invalid_factory",
          path: ["factory"],
          message:
            compatibility.diagnostics[0]?.message ??
            "Factory is not supported by the emulator.",
        },
      ],
    };
  }
  return {
    success: true,
    data: JSON.parse(
      JSON.stringify(reference),
    ) as FactoryEmulatorRuntimeReference,
  };
}

/** Returns detached validated fixtures in their frozen package declaration order. */
export function loadFactoryEmulatorRuntimeReferences(): readonly FactoryEmulatorRuntimeReference[] {
  return runtimeReferenceFixtures.map((fixture) => {
    const parsed = safeParseFactoryEmulatorRuntimeReference(fixture);
    if (!parsed.success)
      throw new Error(
        `Invalid runtime reference ${String((fixture as { id?: unknown }).id)}: ${parsed.issues.map(({ message }) => message).join(" ")}`,
      );
    return parsed.data;
  });
}
