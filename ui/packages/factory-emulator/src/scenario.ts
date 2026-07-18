import type { FactoryDefinition } from "@you-agent-factory/client";
import Ajv2020, { type ErrorObject } from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { generatedScenarioSchema } from "./generated/scenario-schema.js";
import {
  type FactoryEmulatorInitialSubmission,
  type FactoryEmulatorOutcome,
  type FactoryEmulatorRuleSelector,
  type FactoryEmulatorScenario,
  type FactoryEmulatorScenarioIssue,
  type FactoryEmulatorScenarioIssueCode,
  FactoryEmulatorScenarioValidationError,
  type SafeParseFactoryEmulatorScenarioResult,
} from "./scenario-contracts.js";

export * from "./scenario-contracts.js";

export const scenarioSchema = generatedScenarioSchema;

const ajv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: false,
  strict: false,
  useDefaults: false,
});
addFormats(ajv);
const validateScenarioShape = ajv.compile(generatedScenarioSchema);

function pointerSegments(pointer: string): (string | number)[] {
  if (pointer.length === 0) return [];
  return pointer
    .slice(1)
    .split("/")
    .map((segment) => segment.replaceAll("~1", "/").replaceAll("~0", "~"))
    .map((segment) =>
      /^(0|[1-9]\d*)$/.test(segment) ? Number(segment) : segment,
    );
}

function errorPath(error: ErrorObject): readonly (string | number)[] {
  const path = pointerSegments(error.instancePath);
  if (error.keyword === "required") {
    return [...path, error.params.missingProperty];
  }
  if (error.keyword === "additionalProperties") {
    return [...path, error.params.additionalProperty];
  }
  return path;
}

function structuralIssueCode(
  error: ErrorObject,
  path: readonly (string | number)[],
): FactoryEmulatorScenarioIssueCode {
  if (error.keyword === "required") return "missing_required_field";
  if (error.keyword === "additionalProperties") return "unsupported_field";
  if (error.keyword === "type") return "invalid_type";
  if (path.join(".") === "schemaVersion") return "unsupported_schema_version";
  if (path.join(".") === "startAt") return "invalid_start_at";
  if (
    error.keyword === "pattern" &&
    ["id", "name"].includes(String(path.at(-1)))
  ) {
    return "unstable_identity";
  }
  if (path.includes("cursor")) return "invalid_cursor";
  if (path.at(-1) === "exhaustion") return "invalid_exhaustion";
  if (path.includes("unmatched")) return "invalid_unmatched";
  if (path.includes("outcomes") || path.at(-1) === "outcome") {
    return "invalid_outcome";
  }
  return "invalid_value";
}

function structuralIssues(
  errors: readonly ErrorObject[] | null | undefined,
): FactoryEmulatorScenarioIssue[] {
  const results: FactoryEmulatorScenarioIssue[] = [];
  const seen = new Set<string>();
  for (const error of errors ?? []) {
    if (error.keyword === "oneOf") continue;
    const path = errorPath(error);
    const code = structuralIssueCode(error, path);
    const field = path.at(-1);
    const message =
      error.keyword === "required"
        ? `Expected required field ${String(field)}.`
        : error.keyword === "additionalProperties"
          ? `Unsupported field ${String(field)}.`
          : `Expected ${path.join(".") || "scenario"} to satisfy the scenario contract: ${error.message ?? error.keyword}.`;
    const key = `${code}:${path.join(".")}:${message}`;
    if (!seen.has(key)) {
      seen.add(key);
      results.push({ category: "structure", code, path, message });
    }
  }
  return results;
}

function addDuplicateIssues(
  values: readonly string[],
  pathForIndex: (index: number) => readonly (string | number)[],
  label: string,
  issues: FactoryEmulatorScenarioIssue[],
): void {
  const indexesByValue = new Map<string, number[]>();
  for (const [index, value] of values.entries()) {
    const indexes = indexesByValue.get(value) ?? [];
    indexes.push(index);
    indexesByValue.set(value, indexes);
  }
  for (const [value, indexes] of indexesByValue) {
    if (indexes.length < 2) continue;
    for (const index of indexes) {
      issues.push({
        category: "semantic",
        code: "duplicate_identity",
        path: pathForIndex(index),
        message: `${label} identity ${value} is duplicated at indexes ${indexes.join(", ")}.`,
      });
    }
  }
}

function isNormalizedUtcTimestamp(value: string): boolean {
  const timestamp = Date.parse(value);
  return (
    Number.isFinite(timestamp) && new Date(timestamp).toISOString() === value
  );
}

function selectorCovers(
  earlier: FactoryEmulatorRuleSelector,
  later: FactoryEmulatorRuleSelector,
): boolean {
  const earlierValues = [
    earlier.workstation,
    earlier.worker,
    earlier.input?.workType,
    earlier.input?.state,
    earlier.input?.name,
  ];
  const laterValues = [
    later.workstation,
    later.worker,
    later.input?.workType,
    later.input?.state,
    later.input?.name,
  ];
  return earlierValues.every(
    (value, index) => value === undefined || value === laterValues[index],
  );
}

function validateOutcome(
  outcome: FactoryEmulatorOutcome,
  path: readonly (string | number)[],
  issues: FactoryEmulatorScenarioIssue[],
): void {
  if (!Number.isFinite(outcome.durationMs) || outcome.durationMs < 0) {
    issues.push({
      category: "semantic",
      code: "invalid_outcome",
      path: [...path, "durationMs"],
      message: "Outcome durationMs must be a non-negative finite number.",
    });
  }
  const hasError = outcome.error !== undefined;
  if (outcome.result === "failed" ? !hasError : hasError) {
    issues.push({
      category: "semantic",
      code: "invalid_outcome",
      path: [...path, "error"],
      message:
        outcome.result === "failed"
          ? "A failed outcome requires an error."
          : `An ${outcome.result} outcome must not declare an error.`,
    });
  }
}

function validateSelectorReferences(
  scenario: FactoryEmulatorScenario,
  factory: FactoryDefinition,
  issues: FactoryEmulatorScenarioIssue[],
): void {
  const workstationByName = new Map(
    (factory.workstations ?? []).map((workstation) => [
      workstation.name,
      workstation,
    ]),
  );
  const workerNames = new Set(
    (factory.workers ?? []).map((worker) => worker.name),
  );
  const workTypes = new Map(
    (factory.workTypes ?? []).map((workType) => [
      workType.name,
      new Set(workType.states.map((state) => state.name)),
    ]),
  );

  for (const [index, rule] of scenario.rules.entries()) {
    const path = ["rules", index, "selector"] as const;
    const workstation = rule.selector.workstation;
    const worker = rule.selector.worker;
    const workType = rule.selector.input?.workType;
    const state = rule.selector.input?.state;
    if (workstation !== undefined && !workstationByName.has(workstation)) {
      issues.push(
        referenceIssue([...path, "workstation"], "workstation", workstation),
      );
    }
    if (worker !== undefined && !workerNames.has(worker)) {
      issues.push(referenceIssue([...path, "worker"], "worker", worker));
    }
    if (
      workstation !== undefined &&
      worker !== undefined &&
      workstationByName.get(workstation)?.worker !== worker
    ) {
      issues.push({
        category: "semantic",
        code: "invalid_selector_reference",
        path: [...path, "worker"],
        message: `Worker ${worker} is not assigned to workstation ${workstation}.`,
      });
    }
    if (workType !== undefined && !workTypes.has(workType)) {
      issues.push(
        referenceIssue([...path, "input", "workType"], "Work type", workType),
      );
    }
    if (state !== undefined && workType === undefined) {
      issues.push({
        category: "semantic",
        code: "invalid_selector_reference",
        path: [...path, "input", "state"],
        message: "A selector state requires an input Work type.",
      });
    } else if (
      state !== undefined &&
      !workTypes.get(workType ?? "")?.has(state)
    ) {
      issues.push(
        referenceIssue([...path, "input", "state"], "Work state", state),
      );
    }
  }

  for (const [index, submission] of (
    scenario.initialSubmissions ?? []
  ).entries()) {
    const path = ["initialSubmissions", index] as const;
    if (!workTypes.has(submission.workType)) {
      issues.push(
        referenceIssue([...path, "workType"], "Work type", submission.workType),
      );
    } else if (!workTypes.get(submission.workType)?.has(submission.state)) {
      issues.push(
        referenceIssue([...path, "state"], "Work state", submission.state),
      );
    }
  }
}

function referenceIssue(
  path: readonly (string | number)[],
  label: string,
  value: string,
): FactoryEmulatorScenarioIssue {
  return {
    category: "semantic",
    code: "invalid_selector_reference",
    path,
    message: `${label} ${value} is not declared by the Factory.`,
  };
}

function validateSubmissionRelationships(
  submissions: readonly FactoryEmulatorInitialSubmission[],
  issues: FactoryEmulatorScenarioIssue[],
): void {
  const names = new Set(submissions.map((submission) => submission.name));
  const parentByName = new Map(
    submissions
      .filter((submission) => submission.parent !== undefined)
      .map((submission) => [submission.name, submission.parent as string]),
  );
  for (const [index, submission] of submissions.entries()) {
    if (submission.parent === undefined) continue;
    if (!names.has(submission.parent)) {
      issues.push(
        relationshipIssue(
          index,
          `Parent ${submission.parent} is not in this submission batch.`,
        ),
      );
      continue;
    }
    const visited = new Set([submission.name]);
    let parent: string | undefined = submission.parent;
    while (parent !== undefined) {
      if (visited.has(parent)) {
        issues.push(
          relationshipIssue(
            index,
            "Initial submission parent relationships must be acyclic.",
          ),
        );
        break;
      }
      visited.add(parent);
      parent = parentByName.get(parent);
    }
  }
}

function relationshipIssue(
  index: number,
  message: string,
): FactoryEmulatorScenarioIssue {
  return {
    category: "semantic",
    code: "invalid_initial_submission_relationship",
    path: ["initialSubmissions", index, "parent"],
    message,
  };
}

function semanticIssues(
  scenario: FactoryEmulatorScenario,
  factory: FactoryDefinition,
): FactoryEmulatorScenarioIssue[] {
  const issues: FactoryEmulatorScenarioIssue[] = [];
  if (scenario.factory.name !== factory.name) {
    issues.push({
      category: "semantic",
      code: "invalid_factory_identity",
      path: ["factory", "name"],
      message: `Scenario Factory ${scenario.factory.name} does not match ${factory.name}.`,
    });
  }
  if (!isNormalizedUtcTimestamp(scenario.startAt)) {
    issues.push({
      category: "semantic",
      code: "invalid_start_at",
      path: ["startAt"],
      message:
        "startAt must be a normalized UTC RFC 3339 timestamp with millisecond precision.",
    });
  }
  addDuplicateIssues(
    scenario.rules.map((rule) => rule.id),
    (index) => ["rules", index, "id"],
    "Rule",
    issues,
  );
  const submissions = scenario.initialSubmissions ?? [];
  addDuplicateIssues(
    submissions.map((submission) => submission.name),
    (index) => ["initialSubmissions", index, "name"],
    "Initial submission",
    issues,
  );
  for (const [index, rule] of scenario.rules.entries()) {
    const shadowingIndex = scenario.rules
      .slice(0, index)
      .findIndex((earlier) => selectorCovers(earlier.selector, rule.selector));
    if (shadowingIndex >= 0) {
      issues.push({
        category: "semantic",
        code: "fully_shadowed_rule",
        path: ["rules", index, "selector"],
        message: `Rule ${rule.id} is fully shadowed by earlier first-match rule ${scenario.rules[shadowingIndex]?.id}.`,
      });
    }
    rule.outcomes.forEach((outcome, outcomeIndex) => {
      validateOutcome(
        outcome,
        ["rules", index, "outcomes", outcomeIndex],
        issues,
      );
    });
  }
  if (scenario.unmatched.behavior === "outcome") {
    validateOutcome(
      scenario.unmatched.outcome,
      ["unmatched", "outcome"],
      issues,
    );
  }
  validateSelectorReferences(scenario, factory, issues);
  validateSubmissionRelationships(submissions, issues);
  return issues;
}

/** Validate an authored scenario without mutating it or the Factory context. */
export function safeParseFactoryEmulatorScenario(
  input: unknown,
  factory: FactoryDefinition,
): SafeParseFactoryEmulatorScenarioResult {
  if (!validateScenarioShape(input)) {
    return {
      success: false,
      issues: structuralIssues(validateScenarioShape.errors),
    };
  }
  const scenario = input as unknown as FactoryEmulatorScenario;
  const issues = semanticIssues(scenario, factory);
  if (issues.length > 0) return { success: false, issues };
  return {
    success: true,
    data: JSON.parse(JSON.stringify(scenario)) as FactoryEmulatorScenario,
  };
}

/** Parse a complete scenario or throw one error containing every diagnostic. */
export function parseFactoryEmulatorScenario(
  input: unknown,
  factory: FactoryDefinition,
): FactoryEmulatorScenario {
  const result = safeParseFactoryEmulatorScenario(input, factory);
  if (!result.success) {
    throw new FactoryEmulatorScenarioValidationError(result.issues);
  }
  return result.data;
}
