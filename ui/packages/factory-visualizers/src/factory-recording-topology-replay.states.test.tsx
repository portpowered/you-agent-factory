import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen, waitFor, within } from "@testing-library/react";
import type { ComponentType, ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryRecordingTopologyReplay,
  type FactoryRecordingTopologyReplayError,
  type FactoryRecordingTopologyReplayMessages,
} from "./factory-recording-topology-replay";
import type { FactoryVisualizerError } from "./visualizer-error";

vi.mock("@xyflow/react", () => ({
  Background: () => null,
  Controls: () => null,
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  ReactFlow: ({
    children,
    nodes,
    nodeTypes,
  }: {
    children: ReactNode;
    nodes: Array<{ data: Record<string, unknown>; id: string; type: string }>;
    nodeTypes: Record<string, ComponentType<{ data: Record<string, unknown> }>>;
  }) => (
    <div data-testid="controlled-topology-renderer">
      {nodes.map((node) => {
        const NodeView = nodeTypes[node.type];
        return NodeView ? <NodeView data={node.data} key={node.id} /> : null;
      })}
      {children}
    </div>
  ),
}));

const messages: FactoryRecordingTopologyReplayMessages = {
  progress: {
    categories: Object.fromEntries(
      ["active", "completed", "failed", "queued", "unclassified"].map(
        (name) => [
          name,
          {
            plural: (count: string) => `${count} ${name} Work`,
            singular: (count: string) => `${count} ${name} Work`,
          },
        ],
      ),
    ) as FactoryRecordingTopologyReplayMessages["progress"]["categories"],
    empty: "No Work recorded.",
    regionLabel: "Work progress",
    title: "Work progress",
    total: (count) => `${count} Work total`,
  },
  regionLabel: "Recorded Factory playback",
  selectedTick: (tick) => `Selected logical tick ${tick}`,
  timeline: {
    alreadyFollowingLatest: "Already current",
    currentMode: "Following current recording",
    disabled: "Playback disabled",
    followLatest: "Follow latest",
    historyMode: "Inspecting history",
    position: (selected, latest) => `Tick ${selected} of ${latest}`,
    regionLabel: "Recording timeline",
    sliderLabel: "Select recording tick",
    title: "Recording timeline",
    unavailable: "Timeline unavailable",
  },
  topology: {
    activeDispatches: (count) => `${count} active Dispatches`,
    empty: "No Factory topology is available.",
    failed: "The Factory topology could not be shown.",
    inactiveDispatches: "No active Dispatch",
    imageFailed: "The annotation image could not be shown.",
    imageLoading: "Loading annotation image.",
    loading: "Loading Factory topology.",
    nodeLabel: (kind, label) => `${kind}: ${label}`,
    regionLabel: "Factory topology replay",
    resourceOccupancy: (occupied, capacity) =>
      `${occupied} of ${capacity} resources occupied`,
    resourceOccupancyUnavailable: "Resource occupancy unavailable",
    retry: "Try again",
    selectedNode: "Selected",
    workStateCount: (count) => `${count} Work in this state`,
    workStateCountUnavailable: "Work count unavailable",
  },
  validationFailed: "The Factory recording could not be validated.",
};

describe("FactoryRecordingTopologyReplay explicit states", () => {
  it("renders explicit loading and validated empty states", () => {
    const { rerender } = render(
      <FactoryRecordingTopologyReplay
        formatNumber={String}
        messages={messages}
        state={{ status: "loading" }}
      />,
    );

    expect(topologyRegion()).toHaveAttribute("aria-busy", "true");
    expect(within(topologyRegion()).getByRole("status")).toHaveTextContent(
      messages.topology.loading,
    );
    expect(screen.queryByTestId("controlled-topology-renderer")).toBeNull();

    rerender(
      <FactoryRecordingTopologyReplay
        formatNumber={String}
        messages={messages}
        state={{ recording: recording(false), status: "ready" }}
      />,
    );

    expect(within(topologyRegion()).getByRole("status")).toHaveTextContent(
      messages.topology.empty,
    );
    expect(
      screen.getByRole("region", { name: messages.progress.regionLabel }),
    ).toHaveAttribute("data-work-progress-total", "0");
    expect(screen.queryByTestId("controlled-topology-renderer")).toBeNull();
  });

  it("contains a controlled projection failure and removes stale ready content", async () => {
    const onError = vi.fn();
    const projectionError = {
      cause: { code: "INVALID_PROJECTION", name: "ProjectionError" },
      kind: "projection" as const,
      message: "The prepared topology projection could not be read.",
      recoverable: true,
    };
    const { rerender } = render(
      <Example onError={onError} recording={recording(true)} />,
    );
    expect(screen.getByTestId("controlled-topology-renderer")).toBeVisible();

    rerender(<Example onError={onError} stateError={projectionError} />);

    expect(screen.getByRole("alert")).toHaveTextContent(
      messages.topology.failed,
    );
    expect(topologyRegion()).toBeVisible();
    expect(screen.queryByTestId("controlled-topology-renderer")).toBeNull();
    expect(
      screen.queryByRole("region", { name: messages.progress.regionLabel }),
    ).toBeNull();
    expect(screen.getByText("Sibling content survives")).toBeVisible();
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
    expect(onError).toHaveBeenCalledWith(projectionError);
  });
});

function Example({
  onError,
  recording: value,
  stateError,
}: {
  onError: (error: FactoryRecordingTopologyReplayError) => void;
  recording?: ReturnType<typeof recording>;
  stateError?: FactoryVisualizerError;
}) {
  return (
    <>
      <FactoryRecordingTopologyReplay
        formatNumber={String}
        messages={messages}
        onError={onError}
        {...(stateError
          ? { state: { error: stateError, status: "failed" as const } }
          : { recording: value })}
      />
      <p>Sibling content survives</p>
    </>
  );
}

function topologyRegion() {
  return screen.getByRole("region", { name: messages.topology.regionLabel });
}

function recording(withWorkstation: boolean) {
  const factory = {
    name: "local-recording",
    ...(withWorkstation
      ? {
          workstations: [
            { inputs: [], name: "review", outputs: [], worker: "" },
          ],
        }
      : {}),
  };
  return {
    events: [
      {
        context: {
          eventTime: "2026-07-18T19:00:00Z",
          sequence: 1,
          sessionId: "states-session",
          sessionSequence: 1,
          tick: 0,
        },
        id: "states-topology",
        payload: { factory },
        schemaVersion: "agent-factory.event.v1" as const,
        type: "INITIAL_STRUCTURE_REQUEST" as const,
      },
    ],
    factory,
    id: withWorkstation ? "ready-recording" : "empty-recording",
    schemaVersion: "factory-recording/v1" as const,
    title: withWorkstation ? "Ready local recording" : "Empty local recording",
  };
}
