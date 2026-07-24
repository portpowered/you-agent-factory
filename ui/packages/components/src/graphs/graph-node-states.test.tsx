// @vitest-environment happy-dom

import { ReactFlowProvider } from "@xyflow/react";
import {
  GraphNodeButton,
  type GraphNodeHandle,
  GraphNodeShell,
  type GraphNodeState,
} from "@you-agent-factory/components/graphs";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { renderPackageComponent, screen, userEvent } from "../testing/render";

const genericHandles: GraphNodeHandle[] = [
  {
    id: "input-target",
    label: "Input",
    side: "left",
    tone: "input",
    type: "target",
  },
];

function renderNode(ui: ReactElement) {
  return renderPackageComponent(<ReactFlowProvider>{ui}</ReactFlowProvider>);
}

function renderShell(state: GraphNodeState, label = "Example node") {
  return renderNode(
    <GraphNodeShell handles={genericHandles} nodeKind="example" state={state}>
      <GraphNodeButton>{label}</GraphNodeButton>
    </GraphNodeShell>,
  );
}

describe("graph node states", () => {
  it("renders selected shell state with non-color-only emphasis and aria-selected", () => {
    renderShell("selected", "Selected node");

    const shell = document.querySelector('[data-graph-node-kind="example"]');
    expect(shell).toHaveAttribute("data-graph-node-state", "selected");
    expect(shell).toHaveAttribute("aria-selected", "true");
    expect(shell?.className).toContain("border-primary");
    expect(shell?.className).toContain("shadow-[");
  });

  it("renders error shell state with dashed border, alert text, and aria-invalid", () => {
    renderNode(
      <GraphNodeShell
        handles={genericHandles}
        state="error"
        stateLabel="Connection failed"
      >
        <GraphNodeButton>Error node</GraphNodeButton>
      </GraphNodeShell>,
    );

    const shell = document.querySelector('[data-graph-node-state="error"]');
    expect(shell).toHaveAttribute("aria-invalid", "true");
    expect(shell?.className).toContain("border-dashed");
    expect(screen.getByRole("alert")).toHaveTextContent("Connection failed");
  });

  it("renders loading shell state with spinner, aria-busy, and stable content height", () => {
    renderNode(
      <GraphNodeShell handles={genericHandles} state="loading">
        <GraphNodeButton>Loading node</GraphNodeButton>
      </GraphNodeShell>,
    );

    const shell = document.querySelector('[data-graph-node-state="loading"]');
    expect(shell).toHaveAttribute("aria-busy", "true");
    expect(
      document.querySelector('[data-graph-node-loading-spinner="true"]'),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Loading node" }),
    ).toBeInTheDocument();
    expect(shell?.querySelector(".min-h-12")).toBeInTheDocument();
  });

  it("keeps loading and loaded shells at the same content height", () => {
    const { rerender } = renderNode(
      <GraphNodeShell handles={genericHandles} state="loading">
        <GraphNodeButton>Stable node</GraphNodeButton>
      </GraphNodeShell>,
    );

    const loadingShell = document.querySelector(
      '[data-graph-node-state="loading"]',
    );
    const loadingHeight = loadingShell?.getBoundingClientRect().height ?? 0;

    rerender(
      <ReactFlowProvider>
        <GraphNodeShell handles={genericHandles} state="default">
          <GraphNodeButton>Stable node</GraphNodeButton>
        </GraphNodeShell>
      </ReactFlowProvider>,
    );

    const loadedShell = document.querySelector(
      '[data-graph-node-kind="example"]',
    );
    const loadedHeight = loadedShell?.getBoundingClientRect().height ?? 0;

    expect(loadedHeight).toBe(loadingHeight);
  });

  it("renders disabled shell state with aria-disabled", () => {
    renderShell("disabled");

    const shell = document.querySelector('[data-graph-node-state="disabled"]');
    expect(shell).toHaveAttribute("aria-disabled", "true");
  });

  it("prevents disabled graph node button activation while keeping label readable", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();

    renderNode(
      <GraphNodeShell handles={genericHandles}>
        <GraphNodeButton
          graphState="disabled"
          onClick={onClick}
          stateLabel="Disabled node"
        >
          Disabled node
        </GraphNodeButton>
      </GraphNodeShell>,
    );

    const button = screen.getByRole("button", { name: "Disabled node" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-disabled", "true");
    expect(button.className).toContain("cursor-not-allowed");

    await user.click(button);
    expect(onClick).not.toHaveBeenCalled();
    expect(button).toHaveTextContent("Disabled node");
  });

  it("prevents loading graph node button activation and exposes aria-busy", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();

    renderNode(
      <GraphNodeShell handles={genericHandles}>
        <GraphNodeButton
          graphState="loading"
          onClick={onClick}
          stateLabel="Loading node"
        >
          Loading node
        </GraphNodeButton>
      </GraphNodeShell>,
    );

    const button = screen.getByRole("button", { name: "Loading node" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");

    await user.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });

  it("exposes selected graph node button state through aria-pressed", () => {
    renderNode(
      <GraphNodeShell handles={genericHandles}>
        <GraphNodeButton graphState="selected" stateLabel="Selected node">
          Selected node
        </GraphNodeButton>
      </GraphNodeShell>,
    );

    expect(
      screen.getByRole("button", { name: "Selected node" }),
    ).toHaveAttribute("aria-pressed", "true");
  });
});
