import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render } from "@testing-library/react";
import type { ReactElement } from "react";
import { afterEach, beforeEach, vi } from "vitest";
import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../../../api/dashboard/types";
import type { ImportFactoryValue } from "../../../../../api/session-factory";
import { factoryFromDashboardTopology } from "../../../../../components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../../../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useFactoryGraphDraftState } from "../../../../factory-graph-editor/hooks/factory-graph-draft-hook";
import { useEditableFactoryGraph } from "../../../../factory-graph-editor/hooks/use-editable-factory-graph";
import type { ReadFactoryImportFile } from "../../../../import/hooks/use-factory-png-drop";
import type { FactoryImportConfirmInput } from "../../../../import/lib/factory-import-save-choice";
import type { FactoryPngImportValue } from "../../../../import/lib/factory-png-import";
import type { CurrentActivityImportController } from "../../../hooks/current-activity-import-controller";
import { resetCurrentActivityGraphLayoutCacheForTests } from "../../../hooks/react-flow-current-activity-card-graph-layout";
import type { CurrentActivitySelection } from "../../../lib/react-flow-current-activity-card-types";
import { ReactFlowCurrentActivityCard } from "../../react-flow-current-activity-card";
import { DashboardSessionTestProvider } from "../../../../../testing/dashboard-session-test-provider";
import {
  baseFactoryDefinitionDocument,
  createMockGraphEditorDraftState,
  wireMockEditableFactoryGraph,
} from "../../../../../testing/graph-editor-harness";

export const PADDING_CLASS_PATTERN = /(^|\s)p[trblxy]?-[^\s]+/;

export interface RenderCurrentActivityOptions {
  activateFactory?: (
    input: FactoryImportConfirmInput,
  ) => Promise<ImportFactoryValue>;
  currentFactoryDocument?: typeof baseFactoryDefinitionDocument | null;
  currentFactoryDocumentStatus?: "error" | "pending" | "success";
  importController?: CurrentActivityImportController;
  locale?: string;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  snapshot: DashboardSnapshot;
  selection?: CurrentActivitySelection | null;
  widgetInstanceID?: string;
}
export const defaultDraftState = createMockGraphEditorDraftState();

export function refreshFactoryFromTopology(
  snapshot: DashboardSnapshot,
): DashboardSnapshot {
  snapshot.factory = factoryFromDashboardTopology(snapshot.topology);
  return snapshot;
}

export function currentFactoryDocumentFromSnapshot(
  snapshot: DashboardSnapshot,
): typeof baseFactoryDefinitionDocument {
  if (!snapshot.factory) {
    return baseFactoryDefinitionDocument;
  }

  return {
    ...snapshot.factory,
    version: baseFactoryDefinitionDocument.version,
  };
}

export function dashboardSnapshotWithEditableFactory(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.factory = structuredClone(baseFactoryDefinitionDocument);
  return snapshot;
}

export function workerDenseSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.runtime.active_executions_by_dispatch_id = {
    "dispatch-draft-active": {
      dispatch_id: "dispatch-draft-active",
      started_at: "2026-05-19T01:10:00Z",
      transition_id: "draft-transition",
      workstation_name: "draft",
      workstation_node_id: "draft",
    },
  };
  snapshot.runtime.active_dispatch_ids = ["dispatch-draft-active"];
  snapshot.runtime.active_workstation_node_ids = ["draft"];
  snapshot.runtime.active_throttle_pauses = [
    {
      affected_worker_types: ["stalled"],
      lane_id: "provider:codex",
      model: "gpt-5",
      paused_until: "2026-05-19T01:15:00Z",
      provider: "OPENAI",
      recover_at: "2026-05-19T01:16:00Z",
    },
  ];
  snapshot.runtime.workstation_requests_by_dispatch_id = {
    "dispatch-review": {
      counts: {
        dispatched_count: 1,
        errored_count: 1,
        responded_count: 1,
      },
      dispatch_id: "dispatch-review",
      request: {},
      response: {
        failure_message: "Provider request failed.",
        outcome: "FAILED",
      },
      transition_id: "review-transition",
      workstation_name: "review",
      workstation_node_id: "review",
      work_items: [],
    },
  };

  return snapshot;
}
export function dashboardSnapshotWithActiveWorkItemCount(
  count: number,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const reviewActivity =
    snapshot.runtime.workstation_activity_by_node_id?.review;
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[
      "dispatch-review-active"
    ];
  const workItems = Array.from(
    { length: count },
    (_, index): DashboardWorkItemRef => {
      const itemNumber = index + 1;

      return {
        display_name: `Active Story ${itemNumber}`,
        trace_id: `trace-active-story-${itemNumber}`,
        work_id: `work-active-story-${itemNumber}`,
        work_type_id: "story",
      };
    },
  );

  if (count === 0) {
    snapshot.runtime.active_dispatch_ids = (
      snapshot.runtime.active_dispatch_ids ?? []
    ).filter((dispatchID) => dispatchID !== "dispatch-review-active");
    snapshot.runtime.active_workstation_node_ids = (
      snapshot.runtime.active_workstation_node_ids ?? []
    ).filter((nodeID) => nodeID !== "review");
    if (snapshot.runtime.active_executions_by_dispatch_id) {
      delete snapshot.runtime.active_executions_by_dispatch_id[
        "dispatch-review-active"
      ];
    }
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = [];
      reviewActivity.active_work_items = [];
      reviewActivity.trace_ids = [];
    }
  } else if (activeExecution) {
    activeExecution.work_items = workItems;
    activeExecution.trace_ids = workItems.map(
      (workItem) => workItem.trace_id ?? workItem.work_id,
    );
    snapshot.runtime.active_dispatch_ids = ["dispatch-review-active"];
    snapshot.runtime.active_workstation_node_ids = ["review"];
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = ["dispatch-review-active"];
      reviewActivity.active_work_items = workItems;
      reviewActivity.trace_ids = workItems.map(
        (workItem) => workItem.trace_id ?? workItem.work_id,
      );
    }
  }

  snapshot.runtime.in_flight_dispatch_count = count;

  const refreshedSnapshot = refreshFactoryFromTopology(snapshot);
  if (refreshedSnapshot.factory) {
    refreshedSnapshot.factory.version = baseFactoryDefinitionDocument.version;
  }
  return refreshedSnapshot;
}
export function renderCurrentActivity({
  activateFactory,
  currentFactoryDocument,
  currentFactoryDocumentStatus = "success",
  importController,
  locale,
  onFactoryActivated,
  onFactoryImportReady,
  readFactoryImportFile,
  snapshot,
  selection = null,
  widgetInstanceID,
}: RenderCurrentActivityOptions) {
  const factoryDocument =
    currentFactoryDocument !== undefined
      ? currentFactoryDocument
      : currentFactoryDocumentFromSnapshot(snapshot);

  vi.mocked(useCurrentFactoryDocument).mockReturnValue({
    data: factoryDocument ?? undefined,
    error: null,
    status: currentFactoryDocumentStatus,
  } as never);

  const onSelectWorkID =
    vi.fn<
      (workID: string, hint?: { dispatchID?: string; nodeID?: string }) => void
    >();
  const onSelectStateNode = vi.fn<(placeId: string) => void>();
  const onSelectDoc = vi.fn<(targetPath: string) => void>();
  const onSelectResource = vi.fn<(resourceName: string) => void>();
  const onSelectWorker = vi.fn<(workerName: string) => void>();
  const onSelectWorkType = vi.fn<(workTypeName: string) => void>();
  const onSelectWorkstation = vi.fn<(nodeId: string) => void>();

  renderWithQueryClient(
    <ReactFlowCurrentActivityCard
      activateFactory={activateFactory}
      importController={importController}
      locale={locale}
      now={Date.parse("2026-04-08T12:00:04Z")}
      onFactoryActivated={onFactoryActivated}
      onFactoryImportReady={onFactoryImportReady}
      onSelectWorkID={onSelectWorkID}
      onSelectDoc={onSelectDoc}
      onSelectResource={onSelectResource}
      onSelectStateNode={onSelectStateNode}
      onSelectWorker={onSelectWorker}
      onSelectWorkType={onSelectWorkType}
      onSelectWorkstation={onSelectWorkstation}
      readFactoryImportFile={readFactoryImportFile}
      selection={selection}
      snapshot={snapshot}
      widgetInstanceID={widgetInstanceID}
    />,
  );

  return {
    onSelectDoc,
    onSelectResource,
    onSelectStateNode,
    onSelectWorkID,
    onSelectWorker,
    onSelectWorkType,
    onSelectWorkstation,
  };
}

export function renderWithQueryClient(view: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTestProvider>{view}</DashboardSessionTestProvider>
    </QueryClientProvider>,
  );
}

export function createFactoryImportValue(): FactoryPngImportValue {
  return {
    factory: {
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: vi.fn(),
    schemaVersion: "portos.agent-factory.png.v1",
  };
}

export function createImportController(
  overrides: Partial<CurrentActivityImportController> = {},
): CurrentActivityImportController {
  return {
    activateImport: vi.fn().mockResolvedValue(undefined),
    activationState: { status: "idle" },
    clearActivationError: vi.fn(),
    clearError: vi.fn(),
    closeImportPreview: vi.fn(),
    dropState: { status: "idle" },
    importPreviewState: { status: "idle" },
    onDragEnter: vi.fn(),
    onDragLeave: vi.fn(),
    onDragOver: vi.fn(),
    onDrop: vi.fn(),
    ...overrides,
  };
}

export function createFileDropTransfer(files: File[]): {
  dataTransfer: {
    dropEffect: string;
    files: File[];
    types: string[];
  };
} {
  return {
    dataTransfer: {
      dropEffect: "none",
      files,
      types: ["Files"],
    },
  };
}
let restoreBrowserTestShims: (() => void) | null = null;

export function registerCurrentActivityCardTestLifecycle(): void {
  beforeEach(() => {
    window.localStorage.clear();
    resetCurrentActivityGraphLayoutCacheForTests();
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    vi.mocked(useFactoryDocumentSave).mockReturnValue({
      error: null,
      isPending: false,
      reset: vi.fn(),
      save: vi.fn(),
      saveAsync: vi.fn(),
    } as never);
    wireMockEditableFactoryGraph(
      {
        useEditableFactoryGraph: vi.mocked(useEditableFactoryGraph),
        useFactoryGraphDraftState: vi.mocked(useFactoryGraphDraftState),
      },
      defaultDraftState,
    );
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.clearAllMocks();
  });
}
