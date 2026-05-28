import { renderHook, waitFor } from "@testing-library/react";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { useCurrentActivityGraphLayout } from "./react-flow-current-activity-card-graph-layout";

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

describe("useCurrentActivityGraphLayout", () => {
  beforeEach(() => {
    mockBuildGraphLayout.mockReset();
    window.localStorage.clear();
  });

  it("returns an empty graph when no canonical factory is available", async () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    snapshot.factory = undefined;

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayout(snapshot),
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
      useCurrentActivityGraphLayout(snapshot),
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
      "workstation:Draft",
    ]);
    expect(result.current.edges.map((edge) => edge.edgeId).sort()).toEqual([
      "worker-assignment:worker:writer->workstation:Draft",
      "worker-resource:resource:gpu->worker:writer",
      "workstation-input:work-state:story:queued->workstation:Draft",
      "workstation-output:workstation:Draft->work-state:story:done",
      "workstation-resource:resource:gpu->workstation:Draft",
    ]);
  });
});

describe("useCurrentActivityGraphLayout legacy routes", () => {
  beforeEach(() => {
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
      useCurrentActivityGraphLayout(snapshot),
    );

    await waitFor(() => {
      expect(result.current.edges.length).toBeGreaterThan(0);
    });

    expect(result.current.edges.map((edge) => edge.edgeId).sort()).toContain(
      "workstation-on-continue:workstation:Draft->work-state:story:retry",
    );
    expect(result.current.edges.map((edge) => edge.edgeId).sort()).toContain(
      "workstation-on-failure:workstation:Draft->work-state:story:failed",
    );
  });
});
