// @component-test-runner vitest: graph handles require the browser test shims.
import { cleanup, render, screen } from "@testing-library/react";
import { type NodeProps, ReactFlowProvider } from "@xyflow/react";
import {
  type FactoryGraphStatePositionNode,
  type FactoryGraphStatePositionNodeData,
  FactoryGraphStatePositionNodeView,
  type FactoryGraphWorkerNode,
  type FactoryGraphWorkerNodeData,
  FactoryGraphWorkerNodeView,
} from "@you-agent-factory/factory-graph";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";

describe("future canonical graph values", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });

  it("keeps unfamiliar work states and worker kinds raw, neutral, and accessible", () => {
    const stateData: FactoryGraphStatePositionNodeData = {
      activeFlow: false,
      activeItemLabels: [],
      handles: [],
      muted: false,
      onSelectStateNode: vi.fn(),
      place: {
        kind: "work_state",
        place_id: "work-state:story:queued",
        state_category: "QUEUED",
        state_value: "queued",
        type_id: "story",
      },
      selectedStateNode: false,
      tokenCount: 0,
    };
    const workerData: FactoryGraphWorkerNodeData = {
      activeFlow: false,
      focused: false,
      handles: [],
      kind: "worker",
      muted: false,
      onSelectWorker: vi.fn(),
      place: {
        place_id: "worker:writer",
        state_value: "writer",
      },
      runnerId: "CODEX",
      selectedWorker: false,
      workerType: "script_worker",
    };

    render(
      <ReactFlowProvider>
        <FactoryGraphStatePositionNodeView
          {...nodeProps<FactoryGraphStatePositionNode>(
            "work-state:story:queued",
            "statePosition",
            stateData,
          )}
        />
        <FactoryGraphWorkerNodeView
          {...nodeProps<FactoryGraphWorkerNode>(
            "worker:writer",
            "worker",
            workerData,
          )}
        />
      </ReactFlowProvider>,
    );

    const stateButton = screen.getByRole("button", {
      name: /Select story:queued state.*QUEUED/,
    });
    const workerButton = screen.getByRole("button", {
      name: /Select writer \(script_worker\) worker/,
    });

    expect(stateButton).toBeTruthy();
    expect(workerButton).toBeTruthy();
    expect(
      stateButton
        .closest("[data-graph-visual-status]")
        ?.getAttribute("data-graph-visual-status"),
    ).toBe("quiet");
    expect(
      workerButton
        .querySelector("[data-graph-semantic-icon]")
        ?.getAttribute("data-graph-semantic-icon"),
    ).toBe("worker");
    expect(
      workerButton
        .closest("[data-graph-visual-status]")
        ?.getAttribute("data-graph-visual-status"),
    ).toBe("quiet");
  });
});

function nodeProps<
  T extends FactoryGraphStatePositionNode | FactoryGraphWorkerNode,
>(id: string, type: T["type"], data: T["data"]): NodeProps<T> {
  return { data, id, selected: false, type } as NodeProps<T>;
}
