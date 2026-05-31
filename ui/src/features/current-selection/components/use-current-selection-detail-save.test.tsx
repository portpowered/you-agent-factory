import { renderHook } from "@testing-library/react";
import type { DashboardSelection } from "../base/state/selection-types";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { useSaveEditableResourceConfiguration } from "../resource-selection/hooks/use-save-editable-resource-configuration";
import { useSaveEditableWorkerConfiguration } from "../worker-selection/hooks/use-save-editable-worker-configuration";
import { useSaveEditableWorkstationConfiguration } from "../workstation-selection/hooks/use-save-editable-workstation-configuration";
import { useCurrentSelectionDetailSave } from "./use-current-selection-detail-save";

vi.mock(
  "../resource-selection/hooks/use-save-editable-resource-configuration",
  () => ({
    useSaveEditableResourceConfiguration: vi.fn(),
  }),
);

vi.mock("../worker-selection/hooks/use-save-editable-worker-configuration", () => ({
  useSaveEditableWorkerConfiguration: vi.fn(),
}));

vi.mock(
  "../workstation-selection/hooks/use-save-editable-workstation-configuration",
  () => ({
    useSaveEditableWorkstationConfiguration: vi.fn(),
  }),
);

function buildIdleSaveHookReturn() {
  return {
    beginSaveConfirmation: vi.fn(),
    canSave: true,
    cancelSaveConfirmation: vi.fn(),
    confirmSave: vi.fn(),
    save: vi.fn(),
    saveState: { status: "idle" as const },
  };
}

function buildCurrentSelectionStub(): CurrentSelectionState {
  return {
    selectResource: vi.fn(),
    selectWorker: vi.fn(),
  } as never;
}

describe("useCurrentSelectionDetailSave", () => {
  beforeEach(() => {
    vi.mocked(useSaveEditableWorkstationConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableWorkerConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableResourceConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
  });

  it("scopes resource saves to the selected resource name", () => {
    const selection: DashboardSelection = {
      kind: "resource",
      resourceName: "agent-slot",
    };

    renderHook(() =>
      useCurrentSelectionDetailSave({
        currentSelection: buildCurrentSelectionStub(),
        editableConfigurationState: { status: "loading" },
        editableResourceConfigurationState: { status: "loading" },
        editableWorkerConfigurationState: { status: "loading" },
        selectedNode: null,
        selectedResourceName: "agent-slot",
        selectedWorkerName: null,
        selection,
      }),
    );

    expect(useSaveEditableResourceConfiguration).toHaveBeenCalledWith(
      expect.objectContaining({
        scopeKey: "agent-slot",
      }),
    );
    expect(useSaveEditableWorkerConfiguration).toHaveBeenCalledWith(
      expect.objectContaining({
        scopeKey: null,
      }),
    );
  });

  it("returns header actions for each editable selection kind", () => {
    const { result } = renderHook(() =>
      useCurrentSelectionDetailSave({
        currentSelection: buildCurrentSelectionStub(),
        editableConfigurationState: { status: "loading" },
        editableResourceConfigurationState: { status: "loading" },
        editableWorkerConfigurationState: { status: "loading" },
        selectedNode: null,
        selectedResourceName: null,
        selectedWorkerName: null,
        selection: null,
      }),
    );

    expect(result.current.resourceHeaderAction).toBeTruthy();
    expect(result.current.workerHeaderAction).toBeTruthy();
    expect(result.current.workstationHeaderAction).toBeTruthy();
  });
});
