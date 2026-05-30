import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "bun:test";
import traceWorkstationPathRegressionReplayText from "../../../../integration/fixtures/trace-workstation-path-regression-replay.jsonl?raw";

vi.mock("../lib/trace-factory-graph-layout", () => ({
  buildTraceFactoryGraphLayoutPositions: async () => new Map(),
}));

vi.mock("@xyflow/react", async () => ({
  Background: () => <div data-testid="trace-card-flow-background" />,
  Controls: () => <div data-testid="trace-card-flow-controls" />,
  Handle: () => null,
  MarkerType: { ArrowClosed: "arrowclosed" },
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    children,
    nodeTypes,
    nodes,
    onError,
  }: {
    children?: ReactNode;
    nodeTypes: Record<
      string,
      (props: { data: Record<string, unknown> }) => ReactNode
    >;
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    onError?: (id: string, message: string) => void;
  }) => (
    <div
      data-has-on-error={String(Boolean(onError))}
      data-node-ids={JSON.stringify(nodes.map((node) => node.id))}
      data-testid="trace-card-react-flow"
    >
      {nodes.map((node) => {
        const NodeView = nodeTypes[node.type];
        return (
          <div key={node.id}>
            <NodeView data={node.data} />
          </div>
        );
      })}
      {children}
    </div>
  ),
  applyNodeChanges: (
    _changes: Array<Record<string, unknown>>,
    nodes: Array<Record<string, unknown>>,
  ) => nodes,
}));

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { parseReplayFixtureEvents } from "../../../testing/replay-fixtures";
import { buildFactoryTimelineSnapshot } from "../../timeline/state/factoryTimelineStore";
import { TraceGridBentoCard } from "./trace-grid-card";

const TRACE_WORKSTATION_PATH_REGRESSION_TRACE_ID =
  "trace-654eae955c2f3ae2838381993d3e0739";

const TRACE_WORKSTATION_PATH_REGRESSION_WORK_ID = "work-task-60";

const TRACE_WORKSTATION_PATH_REGRESSION_DISPATCH_IDS = [
  "8a56a3ce-6277-41d8-9bc8-840aa10a8d74",
  "145638a2-67c9-4f2a-8a7d-6297ebcd7a19",
  "a5399deb-dffe-4a6d-9b4f-0310aa988bf2",
  "534f91c4-4e83-4310-b211-dbb3ee3cabd1",
  "be0ca2a8-c4f7-42c2-8bd3-a54c0bd9de25",
  "74d8f3b3-d91b-4bcc-927d-b2643e71bc8a",
  "82d4be6a-68c3-4c94-ad3b-53fd53326015",
] as const;

function nodeIDsForFlow(flow: HTMLElement): string[] {
  return JSON.parse(flow.getAttribute("data-node-ids") ?? "[]") as string[];
}

describe("TraceGridBentoCard replayed world state", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders workstation path nodes from the captured event-stream projection", async () => {
    const events = parseReplayFixtureEvents(
      traceWorkstationPathRegressionReplayText,
    );
    const snapshot = buildFactoryTimelineSnapshot(events, 2201);
    const trace =
      snapshot.tracesByWorkID[TRACE_WORKSTATION_PATH_REGRESSION_WORK_ID];

    if (!trace) {
      throw new Error("Expected replayed world state to include work-task-60.");
    }

    expect(trace.trace_id).toBe(TRACE_WORKSTATION_PATH_REGRESSION_TRACE_ID);
    expect(trace.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual(
      TRACE_WORKSTATION_PATH_REGRESSION_DISPATCH_IDS,
    );

    render(<TraceGridBentoCard state={{ status: "ready", trace }} />);

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const workstationFlow = await waitFor(() => {
      const flow = within(card)
        .getAllByTestId("trace-card-react-flow")
        .find((candidate) =>
          nodeIDsForFlow(candidate).includes(
            TRACE_WORKSTATION_PATH_REGRESSION_DISPATCH_IDS[0],
          ),
        );

      expect(flow).toBeTruthy();
      return flow;
    });

    if (!workstationFlow) {
      throw new Error("Expected replayed workstation flow to render.");
    }

    expect(nodeIDsForFlow(workstationFlow).sort()).toEqual(
      [...TRACE_WORKSTATION_PATH_REGRESSION_DISPATCH_IDS].sort(),
    );
    expect(within(workstationFlow).getByText("plan")).toBeTruthy();
    expect(within(workstationFlow).getByText("setup-workspace")).toBeTruthy();
    expect(within(workstationFlow).getAllByText("process")).toHaveLength(5);
  });
});
