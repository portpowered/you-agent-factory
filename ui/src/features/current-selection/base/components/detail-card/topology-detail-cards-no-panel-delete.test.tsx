import "../../../../../testing/vitest-dom-capabilities.setup";

import { render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrentFactoryDocument } from "../../../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryGraphTopologyEditorBridge } from "../../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import { ResourceDetailCard } from "../../../resource-selection/components/resource-detail-card";
import type { EditableResourceConfigurationState } from "../../../resource-selection/lib/detail-card-types";
import { StateNodeDetailCard } from "../../../work-state-selection/components/state-node-detail";
import type { EditableWorkStateConfigurationState } from "../../../work-state-selection/lib/detail-card-types";
import { WorkTypeDetailCard } from "../../../work-type-selection/components/work-type-detail-card";
import type { EditableWorkTypeConfigurationState } from "../../../work-type-selection/lib/detail-card-types";
import { WorkerDetailCard } from "../../../worker-selection/components/worker-detail-card";
import type { EditableWorkerConfigurationState } from "../../../worker-selection/lib/detail-card-types";
import { WorkstationDetailCard } from "../../../workstation-selection/components/detail-card/workstation-detail-card";
import { DETAIL_CARD_NOW } from "./detail-card-test-helpers";

vi.mock(
  "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

const PANEL_TOPOLOGY_DELETE_BUTTON =
  /^Delete .+ (worker|work type|work state|workstation|resource)$/;

function currentSelectionPanel() {
  return screen.getByRole("article", { name: "Current selection" });
}

function assertNoPanelTopologyDeleteAffordances() {
  const panel = within(currentSelectionPanel());

  expect(panel.queryByRole("heading", { name: "Factory graph" })).toBeNull();

  for (const button of panel.queryAllByRole("button")) {
    const accessibleName = button.textContent?.trim() ?? "";
    expect(accessibleName).not.toMatch(PANEL_TOPOLOGY_DELETE_BUTTON);
  }
}

function mockFactoryDocumentQuery(data: CurrentFactoryDocument) {
  vi.mocked(useCurrentFactoryDocument).mockReturnValue({
    data,
    error: null,
    isError: false,
    isPending: false,
    isSuccess: true,
    status: "success",
  } as never);
}

function buildFactoryDocument(
  overrides?: Partial<CurrentFactoryDocument>,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    resources: [
      {
        capacity: 2,
        name: "agent-slot",
        type: "INVOCATION_SLOT",
      },
    ],
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        id: "review",
        name: "Review",
        worker: "reviewer",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
    ],
    ...overrides,
  };
}

function buildReadyWorkerConfigurationState(): EditableWorkerConfigurationState {
  const factoryDocument = buildFactoryDocument();

  return {
    canSave: false,
    draft: {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      name: "reviewer",
      provider: null,
      type: "MODEL_WORKER",
    },
    hasValidationErrors: false,
    initialValues: {
      args: [],
      body: null,
      command: null,
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      name: "reviewer",
      provider: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames: ["Review"],
    },
    isDirty: false,
    markChangesSaved: vi.fn(),
    onArgsTextChange: vi.fn(),
    onBodyChange: vi.fn(),
    onCommandChange: vi.fn(),
    onExecutorProviderChange: vi.fn(),
    onModelChange: vi.fn(),
    onModelLocalityChange: vi.fn(),
    onModelProviderChange: vi.fn(),
    onNameChange: vi.fn(),
    onProviderChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onTypeChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: factoryDocument,
    status: "ready",
    validationErrors: {},
  };
}

function buildReadyWorkTypeConfigurationState(): EditableWorkTypeConfigurationState {
  const factoryDocument = buildFactoryDocument();

  return {
    baseVersion: factoryDocument.version,
    canSave: false,
    draft: {
      handlingBehavior: null,
      name: "story",
    },
    hasValidationErrors: false,
    initialValues: {
      handlingBehavior: undefined,
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
      workTypeName: "story",
    },
    isDirty: false,
    markChangesSaved: vi.fn(),
    onHandlingBehaviorChange: vi.fn(),
    onNameChange: vi.fn(),
    onResetToLatest: vi.fn(),
    pendingFactoryDefinition: factoryDocument,
    status: "ready",
    validationErrors: {},
  };
}

function buildReadyWorkStateConfigurationState(): EditableWorkStateConfigurationState {
  const factoryDocument = buildFactoryDocument();

  return {
    baseVersion: factoryDocument.version,
    canSave: false,
    draft: {
      name: "queued",
      type: "INITIAL",
    },
    hasValidationErrors: false,
    initialValues: {
      stateName: "queued",
      stateNamesInWorkType: ["queued", "done"],
      stateType: "INITIAL",
      workTypeName: "story",
    },
    isDirty: false,
    markChangesSaved: vi.fn(),
    onNameChange: vi.fn(),
    onResetToLatest: vi.fn(),
    originalStateName: "queued",
    pendingFactoryDefinition: factoryDocument,
    status: "ready",
    validationErrors: {},
    workTypeName: "story",
  };
}

function buildReadyResourceConfigurationState(): EditableResourceConfigurationState {
  const factoryDocument = buildFactoryDocument();

  return {
    baseVersion: factoryDocument.version,
    canSave: false,
    draft: {
      backend: "",
      capacityText: "2",
      loadPolicy: "",
      model: "",
      name: "agent-slot",
      provider: "",
      type: "INVOCATION_SLOT",
    },
    hasValidationErrors: false,
    initialValues: {
      backend: null,
      capacity: 2,
      loadPolicy: null,
      model: null,
      provider: null,
      resourceName: "agent-slot",
      type: "INVOCATION_SLOT",
      workerNames: [],
      workstationNames: [],
    },
    isDirty: false,
    markChangesSaved: vi.fn(),
    onBackendChange: vi.fn(),
    onCapacityChange: vi.fn(),
    onLoadPolicyChange: vi.fn(),
    onModelChange: vi.fn(),
    onNameChange: vi.fn(),
    onProviderChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onTypeChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: factoryDocument,
    status: "ready",
    validationErrors: {},
  };
}

describe("topology detail cards with factory graph editor active", () => {
  const requestNodeRemoval = vi.fn();

  beforeEach(() => {
    requestNodeRemoval.mockReset();
    useFactoryGraphTopologyEditorBridge.setState({
      handlers: {
        blockedRemovalReason: null,
        canInteractWithEditor: true,
        editorMode: true,
        requestNodeRemoval,
      },
    });
    mockFactoryDocumentQuery(buildFactoryDocument());
  });

  afterEach(() => {
    useFactoryGraphTopologyEditorBridge.setState({ handlers: null });
  });

  it("does not expose panel topology delete on WorkerDetailCard", () => {
    render(
      <WorkerDetailCard
        editableConfigurationState={buildReadyWorkerConfigurationState()}
        workerName="reviewer"
      />,
    );

    assertNoPanelTopologyDeleteAffordances();
  });

  it("does not expose panel topology delete on WorkTypeDetailCard", () => {
    render(
      <WorkTypeDetailCard
        editableConfigurationState={buildReadyWorkTypeConfigurationState()}
        workTypeName="story"
      />,
    );

    assertNoPanelTopologyDeleteAffordances();
  });

  it("does not expose panel topology delete on StateNodeDetailCard", () => {
    const selectedState =
      semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review
        .input_places?.[0];
    if (!selectedState) {
      throw new Error("expected work state place fixture");
    }

    render(
      <StateNodeDetailCard
        currentWorkItems={[]}
        editableConfigurationState={buildReadyWorkStateConfigurationState()}
        place={selectedState}
        tokenCount={0}
      />,
    );

    assertNoPanelTopologyDeleteAffordances();
  });

  it("does not expose panel topology delete on WorkstationDetailCard", () => {
    const selectedNode =
      semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;

    render(
      <WorkstationDetailCard
        activeExecutions={[]}
        now={DETAIL_CARD_NOW}
        providerSessions={[]}
        selectedNode={selectedNode}
      />,
    );

    assertNoPanelTopologyDeleteAffordances();
  });

  it("does not expose panel topology delete on ResourceDetailCard", () => {
    render(
      <ResourceDetailCard
        editableConfigurationState={buildReadyResourceConfigurationState()}
        resourceName="agent-slot"
      />,
    );

    assertNoPanelTopologyDeleteAffordances();
  });

  it("does not call requestNodeRemoval from current selection detail cards", () => {
    expect(requestNodeRemoval).not.toHaveBeenCalled();
  });
});
