// @vitest-environment happy-dom

import { axe } from "jest-axe";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installReactFlowTestShims } from "../testing/react-flow-test-shims";
import {
  fireEvent,
  renderPackageComponent,
  screen,
  waitFor,
} from "../testing/render";
import { FactoryTopologyReplay } from "./factory-topology-replay";
import {
  factoryTopologyReplayMessages,
  factoryTopologyReplayProjection,
} from "./factory-topology-replay-fixtures";

let restoreReactFlowShims: (() => void) | undefined;

beforeEach(() => {
  restoreReactFlowShims = installReactFlowTestShims();
});
afterEach(() => {
  restoreReactFlowShims?.();
});

describe("FactoryTopologyReplay controlled rendering", () => {
  it("renders prepared topology, handles, activity, Work State counts, and occupancy", async () => {
    const projection = structuredClone(factoryTopologyReplayProjection);
    const before = structuredClone(projection);
    renderPackageComponent(
      <FactoryTopologyReplay
        formatNumber={(value) => new Intl.NumberFormat("de-DE").format(value)}
        messages={factoryTopologyReplayMessages}
        projection={projection}
      />,
    );

    expect(
      await screen.findByRole("region", { name: "Factory topology" }),
    ).toHaveAttribute("data-factory-topology-state", "ready");
    expect(screen.getByText("Logical tick 42")).toBeVisible();
    expect(screen.getByText("1 active Dispatch")).toBeVisible();
    expect(screen.getByText("1 of 2 occupied")).toBeVisible();
    expect(screen.getByText("1.234 Work")).toBeVisible();
    expect(
      document.querySelector('[data-active-dispatch="true"]'),
    ).toBeInTheDocument();
    expect(
      document.querySelector(
        '[data-node-handle-badge="workstation-input-source"]',
      ),
    ).toBeInTheDocument();
    expect(
      document.querySelector(
        '[data-node-handle-badge="workstation-input-target"]',
      ),
    ).toBeInTheDocument();
    expect(projection).toEqual(before);
  });

  it("emits selection intent and changes visible selection only after a controlled update", async () => {
    const onSelectNode = vi.fn();
    const view = renderPackageComponent(
      <FactoryTopologyReplay
        messages={factoryTopologyReplayMessages}
        onSelectNode={onSelectNode}
        projection={factoryTopologyReplayProjection}
      />,
    );
    const reviewButton = await screen.findByRole("button", { name: "Review" });
    fireEvent.click(reviewButton);
    expect(onSelectNode).toHaveBeenCalledWith("workstation:review");
    expect(reviewButton).not.toHaveAttribute("aria-pressed", "true");

    view.rerender(
      <FactoryTopologyReplay
        messages={factoryTopologyReplayMessages}
        onSelectNode={onSelectNode}
        projection={factoryTopologyReplayProjection}
        selectedNodeId="workstation:review"
      />,
    );
    expect(screen.getByText("Selected")).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Review: Selected" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("replaces controlled activity while preserving stable node and edge identity", async () => {
    const view = renderPackageComponent(
      <FactoryTopologyReplay
        messages={factoryTopologyReplayMessages}
        projection={factoryTopologyReplayProjection}
      />,
    );
    const reviewNode = await waitFor(() => {
      const element = document.querySelector(
        '[data-factory-topology-node-id="workstation:review"]',
      );
      expect(element).toBeInTheDocument();
      return element;
    });
    const edgeID = document
      .querySelector('[data-id="work-state:queued->workstation:review"]')
      ?.getAttribute("data-id");
    const nextProjection = structuredClone(factoryTopologyReplayProjection);
    nextProjection.activity.activeDispatches = [];
    nextProjection.activity.selectedTick = 43;
    nextProjection.topology.selectedTick = 43;
    view.rerender(
      <FactoryTopologyReplay
        messages={factoryTopologyReplayMessages}
        projection={nextProjection}
      />,
    );

    expect(screen.getByText("Logical tick 43")).toBeVisible();
    expect(screen.getByText("No active Dispatches")).toBeVisible();
    expect(
      document.querySelector(
        '[data-factory-topology-node-id="workstation:review"]',
      ),
    ).toBe(reviewNode);
    expect(
      document
        .querySelector('[data-id="work-state:queued->workstation:review"]')
        ?.getAttribute("data-id"),
    ).toBe(edgeID);
  });
});

describe("FactoryTopologyReplay endpoint containment", () => {
  it("contains an invalid endpoint, reports it once, and leaves sibling UI alive", () => {
    const onError = vi.fn();
    const invalid = structuredClone(factoryTopologyReplayProjection);
    invalid.topology.connections[0].target.handleId = "missing-target";
    const view = renderPackageComponent(
      <>
        <p>Sibling remains available</p>
        <FactoryTopologyReplay
          messages={factoryTopologyReplayMessages}
          onError={onError}
          projection={invalid}
        />
      </>,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Unable to display topology",
    );
    expect(screen.getByText("Sibling remains available")).toBeVisible();
    expect(screen.queryByText("missing-target")).not.toBeInTheDocument();
    expect(onError).toHaveBeenCalledTimes(1);
    expect(onError).toHaveBeenCalledWith({
      kind: "endpoint",
      message: "Factory topology contains an invalid connection endpoint.",
      recoverable: false,
    });
    view.rerender(
      <>
        <p>Sibling remains available</p>
        <FactoryTopologyReplay
          messages={factoryTopologyReplayMessages}
          onError={onError}
          projection={invalid}
        />
      </>,
    );
    expect(onError).toHaveBeenCalledTimes(1);
  });

  it("rejects endpoint roles that do not match the rendered handle", () => {
    const invalid = structuredClone(factoryTopologyReplayProjection);
    invalid.topology.connections[0].source.handleId = "work-type-state-target";
    invalid.topology.nodes[0].handles = [
      { id: "work-type-state-target", role: "target" },
    ];
    renderPackageComponent(
      <FactoryTopologyReplay
        messages={factoryTopologyReplayMessages}
        projection={invalid}
      />,
    );
    expect(screen.getByRole("alert")).toBeVisible();
    expect(document.querySelector(".react-flow")).not.toBeInTheDocument();
  });
});

describe("FactoryTopologyReplay accessibility", () => {
  it("has no automated accessibility violations in the ready state", async () => {
    const view = renderPackageComponent(
      <FactoryTopologyReplay
        messages={factoryTopologyReplayMessages}
        onSelectNode={() => undefined}
        projection={factoryTopologyReplayProjection}
      />,
    );
    await screen.findByRole("button", { name: "Review" });

    expect(await axe(view.container)).toHaveNoViolations();
  });
});
