import { renderHook } from "@testing-library/react";
import type { DashboardSelection } from "../../base/state/selection-types";
import { useSaveEditableDocConfiguration } from "../../doc-selection/hooks/use-save-editable-doc-configuration";
import type { CurrentSelectionState } from "../../hooks/core/useCurrentSelection";
import { useSaveEditableResourceConfiguration } from "../../resource-selection/hooks/use-save-editable-resource-configuration";
import { useSaveEditableWorkStateConfiguration } from "../../work-state-selection/hooks/use-save-editable-work-state-configuration";
import { useSaveEditableWorkTypeConfiguration } from "../../work-type-selection/hooks/use-save-editable-work-type-configuration";
import { useSaveEditableWorkerConfiguration } from "../../worker-selection/hooks/use-save-editable-worker-configuration";
import { useSaveEditableWorkstationConfiguration } from "../../workstation-selection/hooks/use-save-editable-workstation-configuration";
import { useCurrentSelectionDetailSave } from "./use-current-selection-detail-save";

vi.mock(
  "../../doc-selection/hooks/use-save-editable-doc-configuration",
  () => ({
    useSaveEditableDocConfiguration: vi.fn(),
  }),
);

vi.mock(
  "../../resource-selection/hooks/use-save-editable-resource-configuration",
  () => ({
    useSaveEditableResourceConfiguration: vi.fn(),
  }),
);

vi.mock(
  "../../worker-selection/hooks/use-save-editable-worker-configuration",
  () => ({
    useSaveEditableWorkerConfiguration: vi.fn(),
  }),
);

vi.mock(
  "../../workstation-selection/hooks/use-save-editable-workstation-configuration",
  () => ({
    useSaveEditableWorkstationConfiguration: vi.fn(),
  }),
);

vi.mock(
  "../../work-state-selection/hooks/use-save-editable-work-state-configuration",
  () => ({
    useSaveEditableWorkStateConfiguration: vi.fn(),
  }),
);

vi.mock(
  "../../work-type-selection/hooks/use-save-editable-work-type-configuration",
  () => ({
    useSaveEditableWorkTypeConfiguration: vi.fn(),
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
    selectDoc: vi.fn(),
    selectResource: vi.fn(),
    selectStateNode: vi.fn(),
    selectWorkstation: vi.fn(),
    selectWorkType: vi.fn(),
    selectWorker: vi.fn(),
  } as never;
}

function buildHookArgs(
  overrides: Partial<Parameters<typeof useCurrentSelectionDetailSave>[0]> = {},
) {
  return {
    currentSelection: buildCurrentSelectionStub(),
    editableConfigurationState: { status: "loading" },
    editableDocConfigurationState: { status: "loading" },
    editableResourceConfigurationState: { status: "loading" },
    editableWorkStateConfigurationState: { status: "loading" },
    editableWorkerConfigurationState: { status: "loading" },
    editableWorkTypeConfigurationState: { status: "loading" },
    selectedDocTargetPath: null,
    selectedNode: null,
    selectedResourceName: null,
    selectedWorkerName: null,
    selectedWorkTypeName: null,
    selection: null,
    workStatePlaceId: null,
    ...overrides,
  };
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
    vi.mocked(useSaveEditableWorkStateConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableWorkTypeConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableDocConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
  });

  it("scopes workstation saves to the selected node and wires rename selection refresh", () => {
    const selectWorkstation = vi.fn();
    const selection: DashboardSelection = {
      kind: "node",
      nodeId: "review",
    };

    renderHook(() =>
      useCurrentSelectionDetailSave(
        buildHookArgs({
          currentSelection: {
            ...buildCurrentSelectionStub(),
            selectWorkstation,
          },
          selectedNode: {
            node_id: "review",
            transition_id: "transition",
            workstation_name: "Review",
          } as never,
          selection,
        }),
      ),
    );

    expect(useSaveEditableWorkstationConfiguration).toHaveBeenCalledWith(
      expect.objectContaining({
        onWorkstationRenamed: selectWorkstation,
        scopeKey: "review:transition:Review",
      }),
    );
  });

  it("scopes resource saves to the selected resource name", () => {
    const selection: DashboardSelection = {
      kind: "resource",
      resourceName: "agent-slot",
    };

    renderHook(() =>
      useCurrentSelectionDetailSave(
        buildHookArgs({
          selectedResourceName: "agent-slot",
          selection,
        }),
      ),
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
});

describe("useCurrentSelectionDetailSave additional scopes", () => {
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
    vi.mocked(useSaveEditableWorkStateConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableWorkTypeConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableDocConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
  });

  it("scopes work state saves to the selected place id", () => {
    const selection: DashboardSelection = {
      kind: "state-node",
      placeId: "review-ready",
    };

    renderHook(() =>
      useCurrentSelectionDetailSave(
        buildHookArgs({
          selection,
          workStatePlaceId: "review-ready",
        }),
      ),
    );

    expect(useSaveEditableWorkStateConfiguration).toHaveBeenCalledWith(
      expect.objectContaining({
        scopeKey: "review-ready",
      }),
    );
  });

  it("scopes work type saves to the selected work type name", () => {
    const selection: DashboardSelection = {
      kind: "work-type",
      workTypeName: "inspection",
    };

    renderHook(() =>
      useCurrentSelectionDetailSave(
        buildHookArgs({
          selectedWorkTypeName: "inspection",
          selection,
        }),
      ),
    );

    expect(useSaveEditableWorkTypeConfiguration).toHaveBeenCalledWith(
      expect.objectContaining({
        scopeKey: "inspection",
      }),
    );
  });

  it("scopes doc saves to the selected doc target path", () => {
    const selection: DashboardSelection = {
      kind: "doc",
      targetPath: "factory/docs/overview.md",
    };

    renderHook(() =>
      useCurrentSelectionDetailSave(
        buildHookArgs({
          selectedDocTargetPath: "factory/docs/overview.md",
          selection,
        }),
      ),
    );

    expect(useSaveEditableDocConfiguration).toHaveBeenCalledWith(
      expect.objectContaining({
        scopeKey: "factory/docs/overview.md",
      }),
    );
  });
});

describe("useCurrentSelectionDetailSave header actions", () => {
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
    vi.mocked(useSaveEditableWorkStateConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableWorkTypeConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
    vi.mocked(useSaveEditableDocConfiguration).mockReturnValue(
      buildIdleSaveHookReturn(),
    );
  });

  it("returns header actions for each editable selection kind", () => {
    const { result } = renderHook(() =>
      useCurrentSelectionDetailSave(buildHookArgs()),
    );

    expect(result.current.docHeaderAction).toBeTruthy();
    expect(result.current.resourceHeaderAction).toBeTruthy();
    expect(result.current.workerHeaderAction).toBeTruthy();
    expect(result.current.workstationHeaderAction).toBeTruthy();
    expect(result.current.workStateHeaderAction).toBeTruthy();
    expect(result.current.workTypeHeaderAction).toBeTruthy();
  });
});
