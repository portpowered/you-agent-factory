import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinition,
  currentFactoryDocument,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphLayoutTopologyKey } from "../../factory-graph-editor/lib/operations/factory-graph-topology-impact";
import { currentActivityGraphKey } from "../lib/react-flow-current-activity-card-keys";
import {
  currentActivityCardFactoryDefinition,
  type CurrentActivityFactoryGraphSource,
} from "./current-activity-card-factory-definition";
import { useCurrentActivityGraphLayoutForFactory } from "./react-flow-current-activity-card-graph-layout";
import { useTopologyStableFactoryForLayout } from "./use-topology-stable-factory-for-layout";

function cloneSavedDocument(
  document: CurrentFactoryDocument = currentFactoryDocument,
): CurrentFactoryDocument {
  return structuredClone(document);
}

function createObserverEditorStub(savedDocument: CurrentFactoryDocument) {
  return {
    baseFactoryDocument: savedDocument,
    editableFactoryDocument: savedDocument,
    editableFactoryDocumentStatus: "success" as const,
    editorMode: false,
    latestFactoryDocument: savedDocument,
  } satisfies CurrentActivityFactoryGraphSource;
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
    currentActivityCardFactoryDefinition(editor, snapshot) ?? undefined;
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

function firstWorkstation(document: CurrentFactoryDocument) {
  const workstation = document.workstations?.[0];
  if (!workstation) {
    throw new Error("expected workstation fixture");
  }
  return workstation;
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

async function waitForGraphNodes(result: {
  current: ReturnType<typeof useObserverGraphAfterCurrentSelectionSave>;
}) {
  await waitFor(() => {
    expect(result.current.nodeIds.length).toBeGreaterThan(0);
  });
}

describe("current-selection save graph refresh", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("keeps observer graph layout tied to the event-computed snapshot while saved document data changes", async () => {
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
    const layoutTopologyKeyBeforeSave = result.current.layoutTopologyKey;

    rerender({
      savedDocument: afterWorkstationWorkerAssignmentSave(initialDocument),
    });

    await waitForGraphNodes(result);

    expect([...result.current.nodeIds, ...result.current.edgeIds]).toEqual(
      graphIdsBeforeSave,
    );
    expect(result.current.layoutTopologyKey).toBe(layoutTopologyKeyBeforeSave);
    expect(result.current.layoutTopologyKey).toBe(
      buildFactoryGraphLayoutTopologyKey(snapshot.factory ?? {}),
    );
  });

  it("renders graph nodes from the event snapshot and updates after the event-computed factory changes", async () => {
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

    const { result, rerender } = renderHook(
      (props: {
        savedDocument: CurrentFactoryDocument;
        snapshot: DashboardSnapshot;
      }) => useObserverGraphAfterCurrentSelectionSave(props),
      {
        initialProps: {
          savedDocument,
          snapshot,
        },
      },
    );

    await waitFor(() => {
      expect(result.current.nodeIds).toContain("workstation:stale-only");
    });
    expect(result.current.nodeIds).not.toContain("worker:reviewer");

    rerender({
      savedDocument,
      snapshot: {
        ...snapshot,
        factory: savedDocument,
      },
    });

    await waitFor(() => {
      expect(result.current.nodeIds).toContain("worker:reviewer");
    });
    expect(result.current.nodeIds).not.toContain("workstation:stale-only");
  });
});
