import {
  type FactoryVisualizationLayoutIssue,
  isFactoryVisualizationLayoutCompatibilityIssueCode,
} from "./visualization-layout-contracts.js";

export function partitionFactoryVisualizationLayoutIssues(
  issues: readonly FactoryVisualizationLayoutIssue[],
): {
  blockingIssues: FactoryVisualizationLayoutIssue[];
  diagnostics: FactoryVisualizationLayoutIssue[];
} {
  const blockingIssues: FactoryVisualizationLayoutIssue[] = [];
  const diagnostics: FactoryVisualizationLayoutIssue[] = [];
  for (const issue of issues) {
    (isFactoryVisualizationLayoutCompatibilityIssueCode(issue.code)
      ? diagnostics
      : blockingIssues
    ).push(issue);
  }
  return { blockingIssues, diagnostics };
}
