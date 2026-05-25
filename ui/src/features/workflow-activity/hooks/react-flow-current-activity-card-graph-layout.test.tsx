import { act, renderHook, waitFor } from "@testing-library/react";

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

  it("falls back to the empty graph outcome when a replacement current-activity layout fails", async () => {
    const loadedSnapshot = structuredClone(singleNodeDashboardSnapshot);
    const rejectedSnapshot = structuredClone(singleNodeDashboardSnapshot);
    rejectedSnapshot.topology.workstation_nodes_by_id[
      rejectedSnapshot.topology.workstation_node_ids[0]
    ].workstation_name = "Rejected layout workstation";

    mockBuildGraphLayout.mockImplementation(async (topology) => {
      if (topology === rejectedSnapshot.topology) {
        throw new Error("layout failed");
      }

      if (actualBuildGraphLayoutRef.current === null) {
        throw new Error("expected buildGraphLayout to be available");
      }

      return actualBuildGraphLayoutRef.current(topology);
    });

    const { result, rerender } = renderHook(
      ({ snapshot }) => useCurrentActivityGraphLayout(snapshot),
      {
        initialProps: {
          snapshot: loadedSnapshot,
        },
      },
    );

    await waitFor(() => {
      expect(result.current.nodes.length).toBeGreaterThan(0);
    });

    await act(async () => {
      rerender({ snapshot: rejectedSnapshot });
    });

    await waitFor(() => {
      expect(result.current.nodes).toHaveLength(0);
      expect(result.current.edges).toHaveLength(0);
    });
  });
});
