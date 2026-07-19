import type { FactoryDefinition } from "@you-agent-factory/client";
import type {
  FactoryEmulatorInitialSubmission,
  FactoryEmulatorScenarioIssue,
  FactoryEmulatorSubmissionBatch,
} from "./scenario-contracts.js";

export function validateDependencyRelationships(
  initial: FactoryEmulatorSubmissionBatch,
  factory: FactoryDefinition,
  issues: FactoryEmulatorScenarioIssue[],
): void {
  const relations = initial.relations ?? [];
  const workByName = new Map(initial.works.map((work) => [work.name, work]));
  const targetsBySource = new Map<string, string[]>();
  const relationKeys = new Map<string, number[]>();

  for (const [index, relation] of relations.entries()) {
    const path = ["initialSubmissions", "relations", index] as const;
    if (relation.type !== "DEPENDS_ON") {
      issues.push({
        category: "semantic",
        code: "invalid_initial_submission_relationship",
        path: [...path, "type"],
        message: `Relationship type ${relation.type} is unsupported; the emulator accepts only DEPENDS_ON.`,
      });
    }
    const source = workByName.get(relation.sourceWorkName);
    const target = workByName.get(relation.targetWorkName);
    validateEndpoints(relation, source, target, path, issues);
    validateRequiredState(relation, target, factory, path, issues);

    const key = `${relation.type}\u0000${relation.sourceWorkName}\u0000${relation.targetWorkName}`;
    const indexes = relationKeys.get(key) ?? [];
    indexes.push(index);
    relationKeys.set(key, indexes);
    if (
      relation.type === "DEPENDS_ON" &&
      source !== undefined &&
      target !== undefined &&
      source !== target
    ) {
      const targets = targetsBySource.get(source.name) ?? [];
      targets.push(target.name);
      targetsBySource.set(source.name, targets);
    }
  }

  addDuplicateRelationIssues(relationKeys, issues);
  addCycleIssues(relations, targetsBySource, issues);
}

function validateEndpoints(
  relation: NonNullable<FactoryEmulatorSubmissionBatch["relations"]>[number],
  source: FactoryEmulatorInitialSubmission | undefined,
  target: FactoryEmulatorInitialSubmission | undefined,
  path: readonly (string | number)[],
  issues: FactoryEmulatorScenarioIssue[],
): void {
  if (source === undefined) {
    issues.push({
      category: "semantic",
      code: "invalid_initial_submission_relationship",
      path: [...path, "sourceWorkName"],
      message: `Source Work ${relation.sourceWorkName} is not in this submission batch.`,
    });
  }
  if (target === undefined) {
    issues.push({
      category: "semantic",
      code: "invalid_initial_submission_relationship",
      path: [...path, "targetWorkName"],
      message: `Target Work ${relation.targetWorkName} is not in this submission batch.`,
    });
  }
  if (relation.sourceWorkName === relation.targetWorkName) {
    issues.push({
      category: "semantic",
      code: "invalid_initial_submission_relationship",
      path,
      message: "A Work item cannot depend on itself.",
    });
  }
}

function validateRequiredState(
  relation: NonNullable<FactoryEmulatorSubmissionBatch["relations"]>[number],
  target: FactoryEmulatorInitialSubmission | undefined,
  factory: FactoryDefinition,
  path: readonly (string | number)[],
  issues: FactoryEmulatorScenarioIssue[],
): void {
  if (target === undefined) return;
  const requiredState = relation.requiredState ?? "complete";
  const targetStates = factory.workTypes
    ?.find(({ name }) => name === target.workType)
    ?.states.map(({ name }) => name);
  if (!targetStates?.includes(requiredState)) {
    issues.push({
      category: "semantic",
      code: "invalid_initial_submission_relationship",
      path: [...path, "requiredState"],
      message: `Required state ${requiredState} is not declared by target Work type ${target.workType}.`,
    });
  }
}

function addDuplicateRelationIssues(
  relationKeys: ReadonlyMap<string, readonly number[]>,
  issues: FactoryEmulatorScenarioIssue[],
): void {
  for (const indexes of relationKeys.values()) {
    if (indexes.length < 2) continue;
    for (const index of indexes) {
      issues.push({
        category: "semantic",
        code: "duplicate_identity",
        path: ["initialSubmissions", "relations", index],
        message: `Submission relationship is duplicated at indexes ${indexes.join(", ")}.`,
      });
    }
  }
}

function addCycleIssues(
  relations: NonNullable<FactoryEmulatorSubmissionBatch["relations"]>,
  targetsBySource: ReadonlyMap<string, readonly string[]>,
  issues: FactoryEmulatorScenarioIssue[],
): void {
  const reaches = (
    current: string,
    goal: string,
    visited: Set<string>,
  ): boolean => {
    if (current === goal) return true;
    if (visited.has(current)) return false;
    visited.add(current);
    return (targetsBySource.get(current) ?? []).some((target) =>
      reaches(target, goal, visited),
    );
  };
  relations.forEach((relation, index) => {
    if (
      relation.type === "DEPENDS_ON" &&
      relation.sourceWorkName !== relation.targetWorkName &&
      reaches(relation.targetWorkName, relation.sourceWorkName, new Set())
    ) {
      issues.push({
        category: "semantic",
        code: "invalid_initial_submission_relationship",
        path: ["initialSubmissions", "relations", index],
        message: "DEPENDS_ON relationships must be acyclic.",
      });
    }
  });
}
