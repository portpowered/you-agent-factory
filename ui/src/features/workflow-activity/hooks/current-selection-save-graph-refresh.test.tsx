import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { createMockGraphEditorDraftState } from "../../../testing/graph-editor-harness";
import {
  baseFactoryDefinition,
  currentFactoryDocument,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphLayoutTopologyKey } from "../../factory-graph-editor/lib/operations/factory-graph-topology-impact";
import * as currentActivityFactoryGraphLayout from "../lib/current-activity-factory-graph-layout";
import { currentActivityGraphKey } from "../lib/react-flow-current-activity-card-keys";
import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import { useCurrentActivityGraphLayoutForFactory } from "./react-flow-current-activity-card-graph-layout";
import { currentActivityCardFactoryDefinition } from "./react-flow-current-activity-card-graph-view-model";
import { useTopologyStableFactoryForLayout } from "./use-topology-stable-factory-for-layout";

function cloneSavedDocument(
  document: CurrentFactoryDocument = currentFactoryDocument,
): CurrentFactoryDocument {
  return structuredClone(document);
}

function createObserverEditorStub(savedDocument: CurrentFactoryDocument) {
  return {
    draftState: createMockGraphEditorDraftState({
      baseDocument: savedDocument,
      latestDocument: savedDocument,
    }),
    editableDefinitionQuery: {
      data: savedDocument,
      status: "success" as const,
    },
    editorMode: false,
    hiddenNodeClasses: new Set(),
  } as unknown as ReturnType<typeof useCurrentActivityGraphEditor>;
}

function useObserverGraphAfterCurrentSelectionSave({
  savedDocument,
  snapshot,
}: {
  savedDocument: CurrentFactoryDocument;
  snapshot: DashboardSnapshot;
}) {
  const editor = createObserverEditorStub(savedDocument);
  const displayFactory =
    currentActivityCardFactoryDefinition(editor, snapshot, "current") ??
    undefined;
  const layoutFactory = useTopologyStableFactoryForLayout(displayFactory);
  const graphLayout = useCurrentActivityGraphLayoutForFactory(
    snapshot,
    layoutFactory,
  );

  return {
    displayFactory,
    graphKey: currentActivityGraphKey(graphLayout),
    graphLayout,
    layoutTopologyKey: layoutFactory
      ? buildFactoryGraphLayoutTopologyKey(layoutFactory)
      : "",
    edgeIds: graphLayout.edges.map((edge) => edge.edgeId).sort(),
    nodeIds: graphLayout.nodes.map((node) => node.nodeId).sort(),
  };
}

function buildStaleSnapshotFactoryDocument(
  document: CurrentFactoryDocument = currentFactoryDocument,
): DashboardSnapshot {
  const snapshot = structuredClone(singleNodeDashboardSnapshot);
  snapshot.factory = structuredClone(document);
  snapshot.topology = {
    edges: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  };
  return snapshot;
}

function firstWorkType(document: CurrentFactoryDocument) {
  const workType = document.workTypes?.[0];
  if (!workType) {
    throw new Error("expected work type fixture");
  }
  return workType;
}

function firstWorkstation(document: CurrentFactoryDocument) {
  const workstation = document.workstations?.[0];
  if (!workstation) {
    throw new Error("expected workstation fixture");
  }
  return workstation;
}

function firstWorker(document: CurrentFactoryDocument) {
  const worker = document.workers?.[0];
  if (!worker) {
    throw new Error("expected worker fixture");
  }
  return worker;
}

function afterWorkstationWorkerAssignmentSave(
  previous: CurrentFactoryDocument,
): CurrentFactoryDocument {
  const next = cloneSavedDocument(previous);
  next.workers = [
    ...(next.workers ?? []),
    {
      model: "gpt-5",
      name: "reviewer",
      type: "MODEL_WORKER",
    },
  ];
  next.workstations = [
    {
      ...firstWorkstation(previous),
      worker: "reviewer",
    },
  ];
  next.version = { logical: "6", physical: "2026-06-01T12:00:00Z" };
  return next;
}

function afterWorkerResourceLinkSave(
  previous: CurrentFactoryDocument,
): CurrentFactoryDocument {
  const next = cloneSavedDocument(previous);
  next.workers = [
    {
      ...firstWorker(previous),
      resources: [{ name: "gpu" }],
    },
  ];
  next.version = { logical: "7", physical: "2026-06-01T13:00:00Z" };
  return next;
}

function afterWorkTypeAddSave(
  previous: CurrentFactoryDocument,
): CurrentFactoryDocument {
  const next = cloneSavedDocument(previous);
  next.workTypes = [
    ...(next.workTypes ?? []),
    {
      name: "bug",
      states: [{ name: "open", type: "INITIAL" }],
    },
  ];
  next.version = { logical: "8", physical: "2026-06-01T14:00:00Z" };
  return next;
}

function afterWorkStateAddSave(
  previous: CurrentFactoryDocument,
): CurrentFactoryDocument {
  const next = cloneSavedDocument(previous);
  const workType = firstWorkType(previous);
  next.workTypes = [
    {
      ...workType,
      states: [...workType.states, { name: "review", type: "PROCESSING" }],
    },
  ];
  next.version = { logical: "9", physical: "2026-06-01T15:00:00Z" };
  return next;
}

function afterResourceAddSave(
  previous: CurrentFactoryDocument,
): CurrentFactoryDocument {
  const next = cloneSavedDocument(previous);
  next.resources = [
    ...(next.resources ?? []),
    {
      capacity: 4,
      name: "disk",
    },
  ];
  next.version = { logical: "10", physical: "2026-06-01T16:00:00Z" };
  return next;
}

function afterWorkstationPromptOnlySave(
  previous: CurrentFactoryDocument,
): CurrentFactoryDocument {
  const next = cloneSavedDocument(previous);
  next.workstations = [
    {
      ...firstWorkstation(previous),
      body: "Updated workstation instructions.",
    },
  ];
  next.version = { logical: "11", physical: "2026-06-01T17:00:00Z" };
  return next;
}

async function waitForGraphNodes(result: {
  current: ReturnType<typeof useObserverGraphAfterCurrentSelectionSave>;
}) {
  await waitFor(() => {
    expect(result.current.nodeIds.length).toBeGreaterThan(0);
  });
}

const topologySaveCases = [
  [
    "workstation worker assignment",
    afterWorkstationWorkerAssignmentSave,
    (before: string[], after: string[]) => {
      expect(after).toContain("worker:reviewer");
      expect(after).not.toEqual(before);
    },
  ],
  [
    "worker resource link",
    afterWorkerResourceLinkSave,
    (before: string[], after: string[]) => {
      expect(after).toContain("worker-resource:resource:gpu->worker:writer");
      expect(before).not.toContain(
        "worker-resource:resource:gpu->worker:writer",
      );
    },
  ],
  [
    "work type add",
    afterWorkTypeAddSave,
    (before: string[], after: string[]) => {
      expect(after).toContain("work-type:bug");
      expect(after).not.toEqual(before);
    },
  ],
  [
    "work state add",
    afterWorkStateAddSave,
    (before: string[], after: string[]) => {
      expect(after).toContain("work-state:story:review");
      expect(after).not.toEqual(before);
    },
  ],
  [
    "resource add",
    afterResourceAddSave,
    (before: string[], after: string[]) => {
      expect(after).toContain("resource:disk");
      expect(after).not.toEqual(before);
    },
  ],
] as const;

describe("current-selection save graph refresh", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it.each(
    topologySaveCases,
  )("refreshes observer graph layout after topology-affecting %s save", async (_label, applySave, assertRefresh) => {
    const snapshot = buildStaleSnapshotFactoryDocument();
    const initialDocument = cloneSavedDocument();
    const { result, rerender } = renderHook(
      ({ savedDocument }) =>
        useObserverGraphAfterCurrentSelectionSave({
          savedDocument,
          snapshot,
        }),
      { initialProps: { savedDocument: initialDocument } },
    );

    await waitForGraphNodes(result);
    const graphIdsBeforeSave = [
      ...result.current.nodeIds,
      ...result.current.edgeIds,
    ];

    rerender({
      savedDocument: applySave(initialDocument),
    });

    await waitFor(() => {
      assertRefresh(graphIdsBeforeSave, [
        ...result.current.nodeIds,
        ...result.current.edgeIds,
      ]);
    });
    expect(result.current.layoutTopologyKey).not.toBe(
      buildFactoryGraphLayoutTopologyKey(initialDocument),
    );
  });

  it("keeps graph layout topology key stable after non-topology workstation prompt save", async () => {
    const buildLayoutSpy = vi.spyOn(
      currentActivityFactoryGraphLayout,
      "buildCurrentActivityGraphLayoutFromFactory",
    );
    const snapshot = buildStaleSnapshotFactoryDocument();
    const initialDocument = cloneSavedDocument();
    const promptOnlyDocument = afterWorkstationPromptOnlySave(initialDocument);

    expect(buildFactoryGraphLayoutTopologyKey(initialDocument)).toBe(
      buildFactoryGraphLayoutTopologyKey(promptOnlyDocument),
    );

    const { result, rerender } = renderHook(
      ({ savedDocument }) =>
        useObserverGraphAfterCurrentSelectionSave({
          savedDocument,
          snapshot,
        }),
      { initialProps: { savedDocument: initialDocument } },
    );

    await waitForGraphNodes(result);
    const graphKeyBeforeSave = result.current.graphKey;
    const layoutTopologyKeyBeforeSave = result.current.layoutTopologyKey;
    const callsAfterInitialRender = buildLayoutSpy.mock.calls.length;

    rerender({ savedDocument: promptOnlyDocument });

    await waitForGraphNodes(result);

    expect(result.current.layoutTopologyKey).toBe(layoutTopologyKeyBeforeSave);
    expect(result.current.graphKey).toBe(graphKeyBeforeSave);
    expect(buildLayoutSpy.mock.calls.length).toBe(callsAfterInitialRender);

    buildLayoutSpy.mockRestore();
  });

  it("renders graph nodes from the saved document when the snapshot factory remains stale", async () => {
    const snapshot = buildStaleSnapshotFactoryDocument();
    snapshot.factory = {
      ...structuredClone(baseFactoryDefinition),
      workstations: [
        ...(baseFactoryDefinition.workstations ?? []),
        {
          body: "Stale snapshot workstation.",
          inputs: [{ state: "queued", workType: "story" }],
          name: "stale-only",
          outputs: [{ state: "done", workType: "story" }],
          type: "MODEL_WORKSTATION",
          worker: "writer",
        },
      ],
    };
    const savedDocument = afterWorkstationWorkerAssignmentSave(
      cloneSavedDocument(),
    );

    const { result } = renderHook(() =>
      useObserverGraphAfterCurrentSelectionSave({
        savedDocument,
        snapshot,
      }),
    );

    await waitFor(() => {
      expect(result.current.nodeIds).toContain("worker:reviewer");
    });
    expect(result.current.nodeIds).not.toContain("workstation:stale-only");
  });
});
