import type { FactoryDefinition } from "@you-agent-factory/client";
import { generatedScenarioSchema } from "./generated/scenario-schema.js";
import {
  type FactoryEmulatorInitialSubmission,
  type FactoryEmulatorInitialSubmissions,
  type FactoryEmulatorOutcome,
  type FactoryEmulatorRuleSelector,
  type FactoryEmulatorScenario,
  type FactoryEmulatorScenarioIssue,
  FactoryEmulatorScenarioValidationError,
  type SafeParseFactoryEmulatorScenarioResult,
} from "./scenario-contracts.js";
import { validateFactoryEmulatorScenarioStructure } from "./scenario-validation.js";
import { validateDependencyRelationships } from "./submission-validation.js";

export * from "./scenario-contracts.js";

export const scenarioSchema = generatedScenarioSchema;

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

  const submissions = submissionWorks(scenario.initialSubmissions);
  const workPathPrefix =
    scenario.initialSubmissions !== undefined &&
    isSubmissionArray(scenario.initialSubmissions)
      ? (["initialSubmissions"] as const)
      : (["initialSubmissions", "works"] as const);
  for (const [index, submission] of submissions.entries()) {
    const path = [...workPathPrefix, index] as const;
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

function submissionWorks(
  value: FactoryEmulatorInitialSubmissions | undefined,
): readonly FactoryEmulatorInitialSubmission[] {
  if (value === undefined) return [];
  return isSubmissionArray(value) ? value : value.works;
}

function isSubmissionArray(
  value: FactoryEmulatorInitialSubmissions,
): value is readonly FactoryEmulatorInitialSubmission[] {
  return Array.isArray(value);
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
  pathPrefix: readonly (string | number)[],
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
          pathPrefix,
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
            pathPrefix,
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
  pathPrefix: readonly (string | number)[],
  index: number,
  message: string,
): FactoryEmulatorScenarioIssue {
  return {
    category: "semantic",
    code: "invalid_initial_submission_relationship",
    path: [...pathPrefix, index, "parent"],
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
  const submissions = submissionWorks(scenario.initialSubmissions);
  const submissionPathPrefix =
    scenario.initialSubmissions !== undefined &&
    isSubmissionArray(scenario.initialSubmissions)
      ? (["initialSubmissions"] as const)
      : (["initialSubmissions", "works"] as const);
  addDuplicateIssues(
    submissions.map((submission) => submission.name),
    (index) => [...submissionPathPrefix, index, "name"],
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
  validateSubmissionRelationships(submissions, submissionPathPrefix, issues);
  if (
    scenario.initialSubmissions !== undefined &&
    !isSubmissionArray(scenario.initialSubmissions)
  ) {
    validateDependencyRelationships(
      scenario.initialSubmissions,
      factory,
      issues,
    );
  }
  return issues;
}

/** Validate an authored scenario without mutating it or the Factory context. */
export function safeParseFactoryEmulatorScenario(
  input: unknown,
  factory: FactoryDefinition,
): SafeParseFactoryEmulatorScenarioResult {
  const { blockingIssues: structuralBlocking, diagnostics } =
    validateFactoryEmulatorScenarioStructure(input);
  if (structuralBlocking.length > 0) {
    return {
      success: false,
      issues: structuralBlocking,
      diagnostics,
    };
  }
  const scenario = input as unknown as FactoryEmulatorScenario;
  const issues = semanticIssues(scenario, factory);
  if (issues.length > 0) {
    return { success: false, issues, diagnostics };
  }
  return {
    success: true,
    data: JSON.parse(JSON.stringify(scenario)) as FactoryEmulatorScenario,
    diagnostics,
  };
}

/** Parse a complete scenario or throw one error containing every diagnostic. */
export function parseFactoryEmulatorScenario(
  input: unknown,
  factory: FactoryDefinition,
): FactoryEmulatorScenario {
  const result = safeParseFactoryEmulatorScenario(input, factory);
  if (!result.success) {
    throw new FactoryEmulatorScenarioValidationError(
      result.issues,
      result.diagnostics,
    );
  }
  return result.data;
}
