import { describe, expect, it } from "vitest";

import * as workSelectionPublic from "./index";

describe("work-selection/public", () => {
  it("exports work detail components, execution helpers, and relationship graph builders", () => {
    expect(workSelectionPublic.WorkItemDetailCard).toBeTypeOf("function");
    expect(workSelectionPublic.TerminalWorkSummaryCard).toBeTypeOf("function");
    expect(workSelectionPublic.ExecutionDetailsSection).toBeTypeOf("function");
    expect(workSelectionPublic.InferenceAttemptsSection).toBeTypeOf("function");
    expect(workSelectionPublic.InferenceAttemptCard).toBeTypeOf("function");
    expect(workSelectionPublic.WorkItemPayloadList).toBeTypeOf("function");
    expect(workSelectionPublic.WorkRelationshipsSection).toBeTypeOf("function");
    expect(workSelectionPublic.selectWorkItemExecutionDetails).toBeTypeOf(
      "function",
    );
    expect(workSelectionPublic.buildSelectedWorkRelationshipGraph).toBeTypeOf(
      "function",
    );
    expect(workSelectionPublic).not.toHaveProperty(
      "useSelectedProviderSessionState",
    );
  });
});
