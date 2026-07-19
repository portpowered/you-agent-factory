import "@testing-library/jest-dom/vitest";
import "./testing/vitest.setup";

import { render, screen, waitFor } from "@testing-library/react";
import type { ComponentType, ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryRecordingTopologyReplay,
  type FactoryRecordingTopologyReplayMessages,
} from "./factory-recording-topology-replay";

vi.mock("@xyflow/react", () => ({
  Background: () => null,
  Controls: () => null,
  Handle: ({ id, type }: { id: string; type: string }) => (
    <span data-handle-id={id} data-handle-role={type} />
  ),
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
    categories: {
      active: {
        plural: (count) => `${count} active Work`,
        singular: (count) => `${count} active Work`,
      },
      completed: {
        plural: (count) => `${count} completed Work`,
        singular: (count) => `${count} completed Work`,
      },
      failed: {
        plural: (count) => `${count} failed Work`,
        singular: (count) => `${count} failed Work`,
      },
      queued: {
        plural: (count) => `${count} queued Work`,
        singular: (count) => `${count} queued Work`,
      },
      unclassified: {
        plural: (count) => `${count} unclassified Work`,
        singular: (count) => `${count} unclassified Work`,
      },
    },
    empty: "No Work recorded.",
    regionLabel: "Work progress",
    title: "Work progress",
    total: (count) => `${count} Work total`,
  },
  regionLabel: "Recorded Factory playback",
  selectedTick: (tick) => `Selected logical tick ${tick}`,
  topology: {
    activeDispatches: (count) => `${count} active Dispatches`,
    empty: "No Factory topology is available.",
    failed: "The Factory topology could not be shown.",
    inactiveDispatches: "No active Dispatch",
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

describe("FactoryRecordingTopologyReplay", () => {
  it("validates caller input and delegates selected-tick projections to the shared views", () => {
    const recording = activeRecording();
    const original = structuredClone(recording);

    render(
      <FactoryRecordingTopologyReplay
        formatNumber={(value) => String(value)}
        messages={messages}
        recording={recording}
      />,
    );

    expect(
      screen.getByRole("region", { name: messages.regionLabel }),
    ).toHaveAttribute("data-selected-tick", "3");
    expect(screen.getByText("Selected logical tick 3")).toBeVisible();
    expect(screen.getByTestId("controlled-topology-renderer")).toBeVisible();
    expect(
      document.querySelector('[data-dispatch-activity="active"]'),
    ).toHaveTextContent("1 active Dispatches");
    expect(
      screen.getByRole("figure", { name: "resource: gpu" }),
    ).toHaveTextContent("1 of 2 resources occupied");
    expect(
      screen.getByRole("figure", { name: "work-state: done" }),
    ).toHaveTextContent("0 Work in this state");
    expect(
      screen.getByRole("region", { name: messages.progress.regionLabel }),
    ).toHaveAttribute("data-work-progress-total", "1");
    expect(screen.getByText("1 active Work")).toBeVisible();
    expect(recording).toEqual(original);
  });

  it("contains invalid input, reports one safe diagnostic, and preserves siblings", async () => {
    const onError = vi.fn();
    const invalid = {
      events: [{ payload: { secret: "must-not-leak" } }],
      id: "invalid",
      schemaVersion: "factory-recording/v1",
      title: 42,
    };
    const { rerender } = render(
      <>
        <FactoryRecordingTopologyReplay
          formatNumber={(value) => String(value)}
          messages={messages}
          onError={onError}
          recording={invalid}
        />
        <p>Sibling content survives</p>
      </>,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      messages.topology.failed,
    );
    expect(screen.getByText("Sibling content survives")).toBeVisible();
    expect(
      screen.queryByTestId("controlled-topology-renderer"),
    ).not.toBeInTheDocument();
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({
        issues: expect.arrayContaining([
          expect.objectContaining({ code: "invalid_type", path: ["title"] }),
        ]),
        kind: "recording-validation",
        recoverable: false,
      }),
    );
    expect(JSON.stringify(onError.mock.calls)).not.toContain("must-not-leak");

    rerender(
      <FactoryRecordingTopologyReplay
        formatNumber={(value) => String(value)}
        messages={messages}
        onError={onError}
        recording={invalid}
      />,
    );
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
  });

  it("renders the shared failed presentation when no diagnostic callback is supplied", () => {
    render(
      <FactoryRecordingTopologyReplay
        formatNumber={(value) => String(value)}
        messages={messages}
        recording={null}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      messages.topology.failed,
    );
  });
});

function activeRecording(): unknown {
  const factory = {
    name: "publishing",
    resources: [{ capacity: 2, id: "gpu-stable", name: "gpu" }],
    workers: [{ id: "writer-stable", name: "writer" }],
    workTypes: [
      {
        id: "story-stable",
        name: "story",
        states: [
          { id: "ready-stable", name: "ready", type: "INITIAL" },
          { id: "done-stable", name: "done", type: "TERMINAL" },
        ],
      },
    ],
    workstations: [
      {
        id: "review-stable",
        inputs: [{ state: "ready", workType: "story" }],
        name: "review",
        outputs: [{ state: "done", workType: "story" }],
        resources: [{ capacity: 1, name: "gpu" }],
        worker: "writer",
      },
    ],
  };
  const context = (sequence: number, tick: number, dispatchId?: string) => ({
    ...(dispatchId ? { dispatchId, workIds: ["work-1"] } : {}),
    eventTime: `2026-07-18T23:00:0${sequence}Z`,
    sequence,
    sessionId: "session-1",
    sessionSequence: sequence,
    tick,
  });
  return {
    events: [
      {
        context: context(1, 1),
        id: "topology",
        payload: { factory },
        schemaVersion: "agent-factory.event.v1",
        type: "INITIAL_STRUCTURE_REQUEST",
      },
      {
        context: context(2, 2),
        id: "work",
        payload: {
          type: "FACTORY_REQUEST_BATCH",
          works: [{ name: "Story 1", workId: "work-1", workTypeName: "story" }],
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_REQUEST",
      },
      {
        context: context(3, 3, "dispatch-1"),
        id: "dispatch",
        payload: {
          inputs: [{ workId: "work-1" }],
          resources: [{ capacity: 1, name: "gpu" }],
          transitionId: "review",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "DISPATCH_REQUEST",
      },
    ],
    factory,
    id: "active-recording",
    schemaVersion: "factory-recording/v1",
    title: "Active publishing",
  };
}
