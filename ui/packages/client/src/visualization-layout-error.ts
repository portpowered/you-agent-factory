import type { FactoryVisualizationLayoutIssue } from "./visualization-layout-contracts.js";

export class FactoryVisualizationLayoutValidationError extends Error {
  readonly issues: readonly FactoryVisualizationLayoutIssue[];

  constructor(issues: readonly FactoryVisualizationLayoutIssue[]) {
    super(
      issues.length === 1
        ? `Factory visualization layout validation failed: ${issues[0]?.message}`
        : `Factory visualization layout validation failed with ${issues.length} issues`,
    );
    this.name = "FactoryVisualizationLayoutValidationError";
    this.issues = issues;
  }
}
