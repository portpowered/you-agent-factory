import { act, renderHook, waitFor } from "@testing-library/react";

import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
} from "../../../components/dashboard/test-fixtures";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { useCurrentActivityGraphViewModel } from "./react-flow-current-activity-card";

type BuildGraphLayout = (
  topology: typeof semanticWorkflowDashboardSnapshot.topology,
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

function createEditorStub(overrides: Record<string, unknown> = {}) {
  return {
    activeTool: null,
    canInteractWithEditor: false,
    draftState: {
      draft: {
        additions: {
          resources: [],
          workers: [],
          workStates: [],
          workTypes: [],
          workstations: [],
        },
        edgeChanges: {
          additions: [],
          removals: [],
        },
        removals: {
          resources: [],
          workers: [],
          workStates: [],
          workTypes: [],
          workstations: [],
        },
      },
    },
    editorMode: false,
    handleConnectionAnchorClick: vi.fn(),
    pendingConnectionSource: null,
    ...overrides,
  };
}

describe("useCurrentActivityGraphViewModel", () => {
  beforeEach(() => {
    mockBuildGraphLayout.mockReset();
    window.localStorage.clear();
  });

  it("falls back to the empty graph outcome when a replacement current-activity layout fails", async () => {
    const loadedSnapshot = structuredClone(singleNodeDashboardSnapshot);
    const rejectedSnapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    const onSelectStateNode = vi.fn();
    const onSelectWorkID = vi.fn();
    const onSelectWorkstation = vi.fn();

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
      ({ snapshot }) =>
        useCurrentActivityGraphViewModel({
          editor: createEditorStub() as never,
          now: Date.parse("2026-04-08T12:00:00Z"),
          onSelectStateNode,
          onSelectWorkID,
          onSelectWorkstation,
          selection: null,
          snapshot,
        }),
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
