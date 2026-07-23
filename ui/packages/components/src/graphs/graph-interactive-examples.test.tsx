// @vitest-environment happy-dom

import "@xyflow/react/dist/style.css";

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { installReactFlowTestShims } from "../testing/react-flow-test-shims";
import {
  fireEvent,
  renderPackageComponent,
  screen,
  waitFor,
} from "../testing/render";
import { GraphInteractiveExample } from "./graph-interactive-example";
import { desktopInteractiveGraphNodes } from "./graph-story-fixtures";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: interactive graph cases share one React Flow shim lifecycle.
describe("graph interactive examples", () => {
  let restoreReactFlowShims: (() => void) | undefined;

  beforeEach(() => {
    restoreReactFlowShims = installReactFlowTestShims();
  });

  afterEach(() => {
    restoreReactFlowShims?.();
  });

  it("selects graph nodes on pointer activation and exposes selected state", async () => {
    renderPackageComponent(
      <GraphInteractiveExample fixtureNodes={desktopInteractiveGraphNodes} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Ready node" })).toBeVisible();
    });

    expect(
      screen.getByRole("region", { name: "Interactive graph example" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Connection failed");
    expect(
      document.querySelector('[data-graph-node-state="loading"]'),
    ).toBeInTheDocument();
    expect(
      document.querySelector('[data-graph-node-state="disabled"]'),
    ).toBeInTheDocument();

    const readyButton = screen.getByRole("button", { name: "Ready node" });
    const targetButton = screen.getByRole("button", { name: "Target node" });

    fireEvent.click(readyButton);

    expect(readyButton).toHaveAttribute("aria-pressed", "true");
    expect(
      document.querySelector('[data-graph-node-kind="ready"]'),
    ).toHaveAttribute("aria-selected", "true");
    expect(targetButton).not.toHaveAttribute("aria-pressed", "true");

    fireEvent.click(targetButton);

    expect(targetButton).toHaveAttribute("aria-pressed", "true");
    expect(
      document.querySelector('[data-graph-node-kind="target"]'),
    ).toHaveAttribute("aria-selected", "true");
    expect(readyButton).not.toHaveAttribute("aria-pressed", "true");
  });

  it("activates selectable graph nodes from the keyboard", async () => {
    renderPackageComponent(
      <GraphInteractiveExample fixtureNodes={desktopInteractiveGraphNodes} />,
    );

    const readyButton = await screen.findByRole("button", {
      name: "Ready node",
    });
    readyButton.focus();
    fireEvent.click(readyButton);

    expect(readyButton).toHaveAttribute("aria-pressed", "true");

    const targetButton = screen.getByRole("button", { name: "Target node" });
    targetButton.focus();
    fireEvent.keyDown(targetButton, { key: " ", code: "Space" });
    fireEvent.click(targetButton);

    expect(targetButton).toHaveAttribute("aria-pressed", "true");
    expect(readyButton).not.toHaveAttribute("aria-pressed", "true");
  });

  it("does not activate disabled or loading graph node buttons", async () => {
    renderPackageComponent(
      <GraphInteractiveExample fixtureNodes={desktopInteractiveGraphNodes} />,
    );

    const disabledButton = await screen.findByRole("button", {
      name: "Disabled node",
    });
    const loadingButton = screen.getByRole("button", { name: "Loading node" });

    expect(disabledButton).toBeDisabled();
    expect(loadingButton).toBeDisabled();
    expect(disabledButton).toHaveAttribute("aria-disabled", "true");
    expect(loadingButton).toHaveAttribute("aria-busy", "true");

    fireEvent.click(disabledButton);
    fireEvent.click(loadingButton);

    expect(disabledButton).toBeDisabled();
    expect(loadingButton).toBeDisabled();
  });

  it("renders viewport controls and edge labels in the interactive graph surface", async () => {
    renderPackageComponent(
      <GraphInteractiveExample fixtureNodes={desktopInteractiveGraphNodes} />,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Zoom In" }),
      ).toBeInTheDocument();
    });

    expect(
      screen.getByRole("button", { name: "Zoom Out" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Fit View" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Example edge")).toBeInTheDocument();
    expect(
      document.querySelector('[data-node-handle-badge="input-target"]'),
    ).toBeInTheDocument();
    expect(
      document.querySelector('[data-node-handle-badge="output-source"]'),
    ).toBeInTheDocument();
  });

  it("renders the interactive graph viewport with explicit story height classes", async () => {
    renderPackageComponent(
      <GraphInteractiveExample
        className="h-[28rem]"
        fixtureNodes={desktopInteractiveGraphNodes}
      />,
    );

    const viewport = await screen.findByRole("region", {
      name: "Interactive graph example",
    });

    expect(viewport).toHaveClass("h-[28rem]");
    expect(viewport).not.toHaveClass("h-full");
    expect(viewport).not.toHaveClass("max-h-full");
  });

  it("renders the interactive graph example with an explicit viewport width wrapper", async () => {
    renderPackageComponent(
      <GraphInteractiveExample fixtureNodes={desktopInteractiveGraphNodes} />,
    );

    await screen.findByRole("region", { name: "Interactive graph example" });

    const widthWrapper = document.querySelector(".w-\\[48rem\\]");
    expect(widthWrapper).not.toBeNull();
    expect(widthWrapper).toHaveClass("max-w-full");
  });
});
