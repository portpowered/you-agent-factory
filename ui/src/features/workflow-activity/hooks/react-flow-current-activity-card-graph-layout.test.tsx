import "../../../testing/vitest-dom-capabilities.setup";

import { renderHook, waitFor } from "@testing-library/react";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { buildFactoryGraphLayoutTopologyKey } from "../../factory-graph-editor/lib/operations/factory-graph-topology-impact";
import type { GraphLayout } from "../../flowchart/lib/layout";
import * as currentActivityFactoryGraphLayout from "../lib/current-activity-factory-graph-layout";
import {
  resetCurrentActivityGraphLayoutCacheForTests,
  useCurrentActivityGraphLayoutForFactory,
} from "./react-flow-current-activity-card-graph-layout";

type BuildGraphLayout = (
  topology: typeof singleNodeDashboardSnapshot.topology,
) => Promise<GraphLayout>;

const { actualBuildGraphLayoutRef, mockBuildGraphLayout } = vi.hoisted(() => ({
  actualBuildGraphLayoutRef: { current: null as BuildGraphLayout | null },
  mockBuildGraphLayout: vi.fn(),
}));

vi.mock("../../flowchart/lib/layout", async () => {
  const actual = await vi.importActual("../../flowchart/lib/layout");
  actualBuildGraphLayoutRef.current = actual.buildGraphLayout;

  return {
    ...actual,
    buildGraphLayout: (...args: Parameters<typeof actual.buildGraphLayout>) => {
      const implementation = mockBuildGraphLayout.getMockImplementation();
      if (implementation) {
        return mockBuildGraphLayout(...args);
      }

      return actual.buildGraphLayout(...args);
    },
  };
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: edit-mode layout override cases share one mocked buildGraphLayout harness.
describe("useCurrentActivityGraphLayout", () => {
  beforeEach(() => {
    resetCurrentActivityGraphLayoutCacheForTests();
    mockBuildGraphLayout.mockReset();
    window.localStorage.clear();
  });

  it("returns an empty graph when no canonical factory is available", async () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    snapshot.factory = undefined;

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(snapshot),
    );

    await waitFor(() => {
      expect(result.current.nodes).toHaveLength(0);
      expect(result.current.edges).toHaveLength(0);
    });
    expect(mockBuildGraphLayout).not.toHaveBeenCalled();
  });

  it("builds observer graph layout from the snapshot factory graph when available", async () => {
    const snapshot: DashboardSnapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: {
        name: "canonical-observer",
        resources: [{ capacity: 2, name: "gpu" }],
        workers: [
          {
            model: "gpt-5",
            name: "writer",
            resources: [{ capacity: 1, name: "gpu" }],
            type: "MODEL_WORKER",
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
        workstations: [
          {
            id: "draft",
            inputs: [{ state: "queued", workType: "story" }],
            name: "Draft",
            outputs: [{ state: "done", workType: "story" }],
            resources: [{ capacity: 1, name: "gpu" }],
            type: "MODEL_WORKSTATION",
            worker: "writer",
          },
        ],
      },
      topology: {
        edges: [],
        workstation_node_ids: [],
        workstation_nodes_by_id: {},
      },
    };

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(snapshot),
    );

    await waitFor(() => {
      expect(result.current.nodes.length).toBeGreaterThan(0);
    });

    expect(mockBuildGraphLayout).not.toHaveBeenCalled();
    expect(result.current.nodes.map((node) => node.nodeId).sort()).toEqual([
      "resource:gpu",
      "work-state:story:done",
      "work-state:story:queued",
      "work-type:story",
      "worker:writer",
      "workstation:draft",
    ]);
    expect(result.current.edges.map((edge) => edge.edgeId).sort()).toEqual([
      "worker-assignment:worker:writer->workstation:draft",
      "worker-resource:resource:gpu->worker:writer",
      "workstation-input:work-state:story:queued->workstation:draft",
      "workstation-output:workstation:draft->work-state:story:done",
      "workstation-resource:resource:gpu->workstation:draft",
    ]);
  });

  it("builds edit-mode layout from the document factory override instead of snapshot factory", async () => {
    const snapshot: DashboardSnapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: {
        name: "snapshot-factory",
        workTypes: [
          {
            name: "story",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [
          {
            id: "snapshot-only",
            inputs: [{ state: "queued", workType: "story" }],
            name: "Snapshot Only",
            outputs: [{ state: "done", workType: "story" }],
            type: "MODEL_WORKSTATION",
            worker: "",
          },
        ],
      },
      topology: {
        edges: [],
        workstation_node_ids: [],
        workstation_nodes_by_id: {},
      },
    };
    const documentFactoryOverride: NonNullable<DashboardSnapshot["factory"]> = {
      name: "document-factory",
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          id: "document-only",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Document Only",
          outputs: [{ state: "done", workType: "story" }],
          type: "MODEL_WORKSTATION",
          worker: "",
        },
      ],
    };

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(
        snapshot,
        documentFactoryOverride,
      ),
    );

    await waitFor(() => {
      expect(result.current.nodes.length).toBeGreaterThan(0);
    });

    expect(result.current.nodes.map((node) => node.nodeId).sort()).toContain(
      "workstation:document-only",
    );
    expect(
      result.current.nodes.map((node) => node.nodeId).sort(),
    ).not.toContain("workstation:snapshot-only");
  });

  it("keeps an empty layout when factory override is null even if the snapshot factory is present", async () => {
    const snapshot: DashboardSnapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: {
        name: "snapshot-factory",
        workTypes: [
          {
            name: "story",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [
          {
            id: "snapshot-only",
            inputs: [{ state: "queued", workType: "story" }],
            name: "Snapshot Only",
            outputs: [{ state: "done", workType: "story" }],
            type: "MODEL_WORKSTATION",
            worker: "",
          },
        ],
      },
      topology: {
        edges: [],
        workstation_node_ids: [],
        workstation_nodes_by_id: {},
      },
    };

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(snapshot, null),
    );

    await waitFor(() => {
      expect(result.current.nodes).toHaveLength(0);
      expect(result.current.edges).toHaveLength(0);
    });
    expect(mockBuildGraphLayout).not.toHaveBeenCalled();
  });

  it("reuses cached layout when a non-topology factory override arrives", async () => {
    const buildLayoutSpy = vi.spyOn(
      currentActivityFactoryGraphLayout,
      "buildCurrentActivityGraphLayoutFromFactory",
    );
    const snapshot: DashboardSnapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: {
        name: "prompt-only-cache-test",
        resources: [{ capacity: 2, name: "gpu-prompt-cache" }],
        workers: [
          {
            model: "gpt-5",
            name: "writer-prompt-cache",
            resources: [{ capacity: 1, name: "gpu-prompt-cache" }],
            type: "MODEL_WORKER",
          },
        ],
        workTypes: [
          {
            name: "story-prompt-cache",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [
          {
            body: "Original prompt.",
            id: "draft-prompt-cache",
            inputs: [{ state: "queued", workType: "story-prompt-cache" }],
            name: "Draft Prompt Cache",
            outputs: [{ state: "done", workType: "story-prompt-cache" }],
            resources: [{ capacity: 1, name: "gpu-prompt-cache" }],
            type: "MODEL_WORKSTATION",
            worker: "writer-prompt-cache",
          },
        ],
      },
      topology: {
        edges: [],
        workstation_node_ids: [],
        workstation_nodes_by_id: {},
      },
    };
    const promptOnlyUpdate: NonNullable<DashboardSnapshot["factory"]> = {
      ...structuredClone(snapshot.factory ?? {}),
      workstations: [
        {
          ...(snapshot.factory?.workstations?.[0] ?? {}),
          body: "Updated prompt only.",
        },
      ],
    };

    expect(buildFactoryGraphLayoutTopologyKey(snapshot.factory ?? {})).toBe(
      buildFactoryGraphLayoutTopologyKey(promptOnlyUpdate),
    );

    const { result, rerender } = renderHook(
      ({ factoryOverride }) =>
        useCurrentActivityGraphLayoutForFactory(snapshot, factoryOverride),
      { initialProps: { factoryOverride: snapshot.factory } },
    );

    await waitFor(() => {
      expect(result.current.nodes.length).toBeGreaterThan(0);
    });
    const callsAfterInitialRender = buildLayoutSpy.mock.calls.length;
    expect(callsAfterInitialRender).toBeGreaterThan(0);

    rerender({ factoryOverride: promptOnlyUpdate });

    await waitFor(() => {
      expect(result.current.nodes.length).toBeGreaterThan(0);
    });
    expect(buildLayoutSpy.mock.calls.length).toBe(callsAfterInitialRender);

    buildLayoutSpy.mockRestore();
  });

  it("drops stale resource nodes immediately when factory-change topology removes them", async () => {
    const snapshot: DashboardSnapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: undefined,
      topology: {
        edges: [],
        workstation_node_ids: [],
        workstation_nodes_by_id: {},
      },
    };
    const initialFactory: NonNullable<DashboardSnapshot["factory"]> = {
      name: "factory-stream-delete-regression",
      resources: [
        { capacity: 1, name: "rge" },
        { capacity: 1, name: "asdasd" },
      ],
      version: {
        logical: "1",
        physical: "2026-06-10T10:37:07.833698Z",
      },
    };
    const deletedResourceFactory: NonNullable<DashboardSnapshot["factory"]> = {
      ...initialFactory,
      resources: [{ capacity: 1, name: "asdasd" }],
      version: {
        logical: "2",
        physical: "2026-06-10T10:37:39.734365Z",
      },
    };

    const { result, rerender } = renderHook(
      ({ factoryOverride }) =>
        useCurrentActivityGraphLayoutForFactory(snapshot, factoryOverride),
      {
        initialProps: {
          factoryOverride: initialFactory,
        },
      },
    );

    await waitFor(() => {
      expect(result.current.nodes.map((node) => node.nodeId)).toEqual(
        expect.arrayContaining(["resource:asdasd", "resource:rge"]),
      );
    });

    rerender({ factoryOverride: deletedResourceFactory });

    expect(result.current.nodes.map((node) => node.nodeId)).not.toContain(
      "resource:rge",
    );

    await waitFor(() => {
      expect(result.current.nodes.map((node) => node.nodeId)).toEqual([
        "resource:asdasd",
      ]);
    });
  });
});

describe("useCurrentActivityGraphLayoutForFactory legacy routes", () => {
  beforeEach(() => {
    resetCurrentActivityGraphLayoutCacheForTests();
    mockBuildGraphLayout.mockReset();
    window.localStorage.clear();
  });

  it("accepts legacy singular workstation routes while preserving the canonical factory source", async () => {
    const snapshot: DashboardSnapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: {
        name: "legacy-route-observer",
        workTypes: [
          {
            name: "story",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "retry", type: "PROCESSING" },
              { name: "failed", type: "FAILED" },
            ],
          },
        ],
        workstations: [
          {
            id: "draft",
            inputs: [{ state: "queued", workType: "story" }],
            name: "Draft",
            onContinue: { state: "retry", workType: "story" },
            onFailure: { state: "failed", workType: "story" },
            outputs: [],
            type: "MODEL_WORKSTATION",
            worker: "",
          } as NonNullable<
            DashboardSnapshot["factory"]
          >["workstations"][number],
        ],
      },
      topology: {
        edges: [],
        workstation_node_ids: [],
        workstation_nodes_by_id: {},
      },
    };

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(snapshot),
    );

    await waitFor(() => {
      expect(result.current.edges.length).toBeGreaterThan(0);
    });

    expect(result.current.edges.map((edge) => edge.edgeId).sort()).toContain(
      "workstation-on-continue:workstation:draft->work-state:story:retry",
    );
    expect(result.current.edges.map((edge) => edge.edgeId).sort()).toContain(
      "workstation-on-failure:workstation:draft->work-state:story:failed",
    );
  });
});
