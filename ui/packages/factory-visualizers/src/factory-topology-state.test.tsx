import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { FactoryTopologyReplayMessages } from "./factory-topology-replay";
import {
  FactoryTopologyErrorBoundary,
  FactoryTopologyStateRegion,
  useDistinctTopologyErrorReport,
} from "./factory-topology-state";

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => `${count} active Dispatches`,
  annotationsHidden: "Show annotations",
  annotationsVisible: "Hide annotations",
  empty: "No Factory topology is available.",
  failed: "The Factory topology could not be shown.",
  inactiveDispatches: "No active Dispatch",
  imageFailed: "The annotation image could not be shown.",
  imageLoading: "Loading annotation image.",
  legendActiveRoute: "Active route",
  legendInactiveRoute: "Inactive route",
  legendLabel: "Topology legend",
  loading: "Loading Factory topology.",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology replay",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} resources occupied`,
  resourceOccupancyUnavailable: "Resource occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected",
  viewportControlsLabel: "Topology viewport controls",
  workStateCount: (count) => `${count} Work in this state`,
  workStateCountUnavailable: "Work count unavailable",
};

describe("Factory topology state surfaces", () => {
  it("renders loading and empty states, and provides retry only for failures", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    const { rerender } = render(
      <FactoryTopologyStateRegion messages={messages} state="loading" />,
    );

    expect(screen.getByRole("region")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText(messages.loading)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: messages.retry })).toBeNull();

    rerender(<FactoryTopologyStateRegion messages={messages} state="empty" />);
    expect(screen.getByRole("status")).toHaveTextContent(messages.empty);

    rerender(
      <FactoryTopologyStateRegion
        messages={messages}
        onRetry={onRetry}
        state="failed"
      />,
    );
    await user.click(screen.getByRole("button", { name: messages.retry }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("reports a render error once and resets when its reset key changes", () => {
    const onError = vi.fn();
    const Throw = () => {
      throw new Error("broken topology");
    };
    const { rerender } = render(
      <FactoryTopologyErrorBoundary
        errorKind="render"
        messages={messages}
        onError={onError}
        resetKeys={["first"]}
      >
        <Throw />
      </FactoryTopologyErrorBoundary>,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(messages.failed);
    expect(onError).toHaveBeenCalledOnce();

    rerender(
      <FactoryTopologyErrorBoundary
        errorKind="render"
        messages={messages}
        onError={onError}
        resetKeys={["second"]}
      >
        <span>Recovered topology</span>
      </FactoryTopologyErrorBoundary>,
    );

    expect(screen.getByText("Recovered topology")).toBeInTheDocument();
  });

  it("deduplicates repeated layout diagnostics before reporting them", async () => {
    const user = userEvent.setup();
    const onError = vi.fn();
    const diagnostic = {
      issues: [{ category: "topology", code: "missing-handle", path: ["edge"] }],
      kind: "layout-validation" as const,
      message: "Invalid topology layout",
      recoverable: true as const,
    };
    const Reporter = () => {
      const [error, setError] = useState<typeof diagnostic | undefined>(
        diagnostic,
      );
      useDistinctTopologyErrorReport(error, onError);
      return (
        <button onClick={() => setError({ ...diagnostic })} type="button">
          Report again
        </button>
      );
    };

    render(<Reporter />);
    await user.click(screen.getByRole("button", { name: "Report again" }));

    expect(onError).toHaveBeenCalledOnce();
  });
});
