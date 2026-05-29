import { describe, expect, it } from "vitest";

import * as workstationSelectionPublic from "./index";

describe("workstation-selection/public", () => {
  it("exports workstation detail, save controls, hooks, and message helpers", () => {
    expect(workstationSelectionPublic.WorkstationDetailCard).toBeTypeOf("function");
    expect(workstationSelectionPublic.EditableWorkstationSaveDialog).toBeTypeOf(
      "function",
    );
    expect(workstationSelectionPublic.EditableWorkstationSaveHeaderAction).toBeTypeOf(
      "function",
    );
    expect(workstationSelectionPublic.useEditableWorkstationConfigurationState).toBeTypeOf(
      "function",
    );
    expect(workstationSelectionPublic.useSaveEditableWorkstationConfiguration).toBeTypeOf(
      "function",
    );
    expect(workstationSelectionPublic.getWorkstationDetailMessages).toBeTypeOf(
      "function",
    );
    expect(workstationSelectionPublic.ProviderSessionAttempts).toBeTypeOf("function");
    expect(workstationSelectionPublic.CollapsibleProviderSessionAttempts).toBeTypeOf(
      "function",
    );
  });
});
