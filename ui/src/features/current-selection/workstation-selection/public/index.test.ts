import { describe, expect, it } from "vitest";

import * as workstationSelectionPublic from "./index";

describe("workstation-selection/public", () => {
  it("exports workstation detail, save controls, runner metadata, and message helpers", () => {
    expect(workstationSelectionPublic.WorkstationDetailCard).toBeTypeOf(
      "function",
    );
    expect(workstationSelectionPublic.EditableWorkstationSaveDialog).toBeTypeOf(
      "function",
    );
    expect(
      workstationSelectionPublic.EditableWorkstationSaveHeaderAction,
    ).toBeTypeOf("function");
    expect(workstationSelectionPublic.getWorkstationDetailMessages).toBeTypeOf(
      "function",
    );
    expect(workstationSelectionPublic.ProviderSessionAttempts).toBeTypeOf(
      "function",
    );
    expect(
      workstationSelectionPublic.CollapsibleProviderSessionAttempts,
    ).toBeTypeOf("function");
    expect(
      workstationSelectionPublic.BUILT_IN_RUNNER_IDS.length,
    ).toBeGreaterThan(0);
    expect(workstationSelectionPublic.getRunnerMetadata).toBeTypeOf("function");
  });

  it("does not export workstation editing hooks", () => {
    expect(workstationSelectionPublic).not.toHaveProperty(
      "useEditableWorkstationConfigurationState",
    );
    expect(workstationSelectionPublic).not.toHaveProperty(
      "useSaveEditableWorkstationConfiguration",
    );
    expect(workstationSelectionPublic).not.toHaveProperty(
      "useCurrentWorkstationPromptTemplateValidation",
    );
  });
});
