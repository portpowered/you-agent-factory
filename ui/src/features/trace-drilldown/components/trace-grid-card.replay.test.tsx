import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import traceWorkstationPathRegressionReplayText from "../../../../integration/fixtures/trace-workstation-path-regression-replay.jsonl?raw";

vi.mock("../lib/trace-factory-graph-layout", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../lib/trace-factory-graph-layout")>();
  return {
    ...actual,
    buildTraceFactoryGraphLayoutPositions: async () => new Map(),
  };
});

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

import type { DashboardTrace } from "../../../api/dashboard/types";
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

const RELATION_ONLY_TRACE: DashboardTrace = {
  dispatches: [],
  relations: [
    {
      source_work_id: "work-plan",
      source_work_name: "Plan story",
      target_work_id: "work-implement",
      target_work_name: "Implement story",
      type: "PARENT_CHILD",
    },
  ],
  trace_id: "trace-relation-only",
  transition_ids: [],
  work_ids: ["work-plan", "work-implement"],
  workstation_sequence: [],
};

function nodeIDsForFlow(flow: HTMLElement): string[] {
  return JSON.parse(flow.getAttribute("data-node-ids") ?? "[]") as string[];
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: Replay coverage keeps the captured trace and bidirectional selection scenario together.
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

  it("renders relation-only traces without the known-empty state", async () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: RELATION_ONLY_TRACE }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    await waitFor(() => {
      expect(
        within(card).getByRole("region", { name: "Batch relation graph" }),
      ).toBeTruthy();
    });

    const relationGraph = within(card).getByRole("region", {
      name: "Batch relation graph",
    });
    expect(within(relationGraph).getByText("Plan story")).toBeTruthy();
    expect(within(relationGraph).getByText("Implement story")).toBeTruthy();
    expect(within(card).queryByText("Trace history unavailable")).toBeNull();
    expect(
      within(card).queryByText(
        "No retained dispatch history is currently available for this work item.",
      ),
    ).toBeNull();
  });

  it("synchronizes table and graph selection by the full trace identity", async () => {
    const trace: DashboardTrace = {
      dispatches: [
        {
          attempt: 2,
          dispatch_id: "dispatch-retry",
          duration_millis: 1000,
          end_time: "2026-05-27T14:15:24.172854+07:00",
          input_items: [
            {
              display_name: "Shared work",
              work_id: "work-shared",
              work_type_id: "story",
            },
          ],
          outcome: "ACCEPTED",
          start_time: "2026-05-27T14:13:35.734332+07:00",
          transition_id: "retry",
          workstation_name: "Retry",
          work_ids: ["work-shared"],
        },
      ],
      trace_id: "trace-retry-selection",
      transition_ids: ["retry"],
      work_ids: ["work-shared"],
    };

    render(<TraceGridBentoCard state={{ status: "ready", trace }} />);

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const dispatchButton = await waitFor(() => {
      const button = within(card).getByRole("button", {
        name: "Select dispatch dispatch-retry, Work work-shared, attempt 2.",
      });
      expect(button).toBeTruthy();
      return button;
    });
    const graphSurface = [
      ...card.querySelectorAll<HTMLElement>(
        '[data-trace-selection-surface="graph"]',
      ),
    ].find((candidate) =>
      candidate
        .getAttribute("data-trace-selection-keys")
        ?.includes("dispatch-retry|work-shared|2"),
    );
    if (!graphSurface) {
      throw new Error("Expected a trace graph selection surface.");
    }
    const graphButton = within(graphSurface).getByRole("button", {
      name: "Select Retry workstation",
    });

    fireEvent.click(dispatchButton);
    await waitFor(() => {
      expect(graphButton.getAttribute("aria-pressed")).toBe("true");
      expect(document.activeElement).toBe(graphButton);
    });

    fireEvent.click(graphButton);
    await waitFor(() => {
      expect(dispatchButton.getAttribute("aria-pressed")).toBe("true");
      expect(document.activeElement).toBe(dispatchButton);
    });
  });

  it("returns textual relation selection to the matching dispatch graph item", async () => {
    const trace: DashboardTrace = {
      dispatches: [
        {
          attempt: 1,
          current_chaining_trace_id: "trace-plan-chain",
          dispatch_id: "dispatch-plan",
          duration_millis: 1000,
          end_time: "2026-05-27T14:15:24.172854+07:00",
          output_items: [{ work_id: "work-shared" }],
          outcome: "ACCEPTED",
          start_time: "2026-05-27T14:13:35.734332+07:00",
          transition_id: "plan",
          workstation_name: "Plan",
        },
        {
          attempt: 2,
          dispatch_id: "dispatch-retry",
          duration_millis: 1000,
          end_time: "2026-05-27T14:15:25.614203+07:00",
          input_items: [{ work_id: "work-shared" }],
          outcome: "ACCEPTED",
          start_time: "2026-05-27T14:15:24.183895+07:00",
          transition_id: "retry",
          workstation_name: "Retry",
        },
      ],
      relations: [
        {
          source_work_id: "work-shared",
          source_work_name: "Shared work",
          target_work_id: "work-shared",
          target_work_name: "Shared work",
          type: "RETRY",
        },
      ],
      trace_id: "trace-text-selection",
      transition_ids: ["plan", "retry"],
      work_ids: ["work-shared"],
      workstation_sequence: ["Plan", "Retry"],
    };

    render(<TraceGridBentoCard state={{ status: "ready", trace }} />);

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const textPath = await waitFor(() => {
      const path = within(card)
        .getAllByRole("region", { name: "Textual relation path" })
        .find((candidate) =>
          candidate.querySelector(
            '[data-trace-relation-id^="work-type-state|"]',
          ),
        );
      expect(path).toBeTruthy();
      return path;
    });
    const textSelectionButtons = within(textPath).getAllByRole("button", {
      name: "dispatch-retry · Work work-shared · attempt 2",
    });
    expect(textSelectionButtons).toHaveLength(2);
    const textSelectionButton = textSelectionButtons[1];
    if (!textSelectionButton) {
      throw new Error("Expected the target relation identity to render.");
    }
    const graphSurface = [
      ...card.querySelectorAll<HTMLElement>(
        '[data-trace-selection-surface="graph"]',
      ),
    ].find((candidate) =>
      candidate
        .getAttribute("data-trace-selection-keys")
        ?.includes("dispatch-retry|work-shared|2"),
    );
    if (!graphSurface) {
      throw new Error("Expected a dispatch graph selection surface.");
    }
    const retryGraphButton = within(graphSurface).getByRole("button", {
      name: "Select Retry workstation",
    });

    fireEvent.click(textSelectionButton);
    await waitFor(() => {
      expect(retryGraphButton.getAttribute("aria-pressed")).toBe("true");
      expect(document.activeElement).toBe(retryGraphButton);
    });
  });
});
