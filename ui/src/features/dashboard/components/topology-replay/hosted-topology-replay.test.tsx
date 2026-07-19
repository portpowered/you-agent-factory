import { act, fireEvent, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import type { FactoryVisualizationLayoutV1 } from "@you-agent-factory/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import type { StreamDerivedCacheIdentity } from "../../../timeline/public";
import { useFactoryTimelineStore } from "../../../timeline/public";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../../state/dashboardStreamStore";
import { HostedTopologyReplay } from "./hosted-topology-replay";

const liveStreamState = { message: "Live", status: "live" } as const;
let restoreBrowserShims: (() => void) | undefined;

function identity(
  factorySessionID: string,
  streamGenerationID = "generation-1",
): StreamDerivedCacheIdentity {
  return {
    backendScopeID: "backend-a",
    factorySessionID,
    logicalSessionKeyID: "logical-session",
    streamGenerationID,
  };
}

function topologyEvent({
  id,
  sequence,
  stateName,
  tick,
  workTypeName,
}: {
  id: string;
  sequence: number;
  stateName: string;
  tick: number;
  workTypeName: string;
}): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-19T04:00:${String(sequence).padStart(2, "0")}.000Z`,
      sequence,
      tick,
    },
    id,
    payload: {
      factory: {
        name: `${workTypeName} factory`,
        workers: [],
        workTypes: [
          {
            name: workTypeName,
            states: [{ name: stateName, type: "INITIAL" }],
          },
        ],
        workstations: [],
      },
    },
    type:
      sequence === 1
        ? FACTORY_EVENT_TYPES.initialStructureRequest
        : FACTORY_EVENT_TYPES.factoryChange,
  };
}

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
  act(() => {
    useFactoryTimelineStore.getState().reset();
    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: null,
      resolvedStreamIdentity: null,
      streamState: createDefaultDashboardStreamState(),
    });
  });
});

describe("hosted topology replay rendering", () => {
  it("forwards caller-owned annotations without changing hosted replay", () => {
    const session = identity("session-with-layout");
    act(() => {
      useFactoryTimelineStore.getState().appendEventForEntry(
        session,
        topologyEvent({
          id: "initial",
          sequence: 1,
          stateName: "queued",
          tick: 1,
          workTypeName: "story",
        }),
      );
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: session,
        streamState: liveStreamState,
      });
    });
    const layout: FactoryVisualizationLayoutV1 = {
      annotations: [
        {
          body: "Caller-owned hosted context",
          id: "hosted-context",
          kind: "note",
          position: { x: 720, y: 20 },
          size: { height: 90, width: 180 },
        },
      ],
      schemaVersion: "factory-visualization-layout/v1",
    };

    render(<HostedTopologyReplay layout={layout} />);

    expect(screen.getByText("Caller-owned hosted context")).toBeInTheDocument();
    expect(screen.getByText("queued")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Hide annotations" }));
    expect(screen.queryByText("Caller-owned hosted context")).toBeNull();
    expect(screen.getByText("queued")).toBeInTheDocument();
  });
});

describe("hosted topology replay timeline rendering", () => {
  it("renders current and fixed history from accepted exact-entry tails", async () => {
    const session = identity("session-a");
    act(() => {
      useFactoryTimelineStore.getState().appendEventsForEntry(session, [
        topologyEvent({
          id: "initial",
          sequence: 1,
          stateName: "queued",
          tick: 1,
          workTypeName: "story",
        }),
        topologyEvent({
          id: "same-tick-head",
          sequence: 2,
          stateName: "reviewing",
          tick: 2,
          workTypeName: "story",
        }),
      ]);
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: session,
        streamState: liveStreamState,
      });
    });

    render(<HostedTopologyReplay />);
    expect(screen.getByText("reviewing")).toBeInTheDocument();
    expect(screen.getByText("Following live Factory updates.")).toBeVisible();

    act(() => {
      useFactoryTimelineStore.getState().appendEventForEntry(
        session,
        topologyEvent({
          id: "same-tick-tail",
          sequence: 3,
          stateName: "approved",
          tick: 2,
          workTypeName: "story",
        }),
      );
    });
    expect(await screen.findByText("approved")).toBeInTheDocument();

    const entryAfterTail = useFactoryTimelineStore
      .getState()
      .entryForIdentity(session);
    act(() => {
      useFactoryTimelineStore.getState().appendEventForEntry(
        session,
        topologyEvent({
          id: "same-tick-tail",
          sequence: 4,
          stateName: "duplicate-must-not-render",
          tick: 2,
          workTypeName: "story",
        }),
      );
    });
    expect(useFactoryTimelineStore.getState().entryForIdentity(session)).toBe(
      entryAfterTail,
    );
    expect(screen.queryByText("duplicate-must-not-render")).toBeNull();

    fireEvent.change(
      screen.getByRole("slider", { name: "Select Factory replay tick" }),
      { target: { value: "1" } },
    );
    expect(screen.getByText("queued")).toBeInTheDocument();
    expect(screen.getByText("Viewing historical Factory state.")).toBeVisible();

    act(() => {
      useFactoryTimelineStore.getState().appendEventForEntry(
        session,
        topologyEvent({
          id: "future-tail",
          sequence: 5,
          stateName: "complete",
          tick: 3,
          workTypeName: "story",
        }),
      );
    });
    expect(screen.getByText("queued")).toBeInTheDocument();
    expect(screen.getByText("Tick 1 of Tick 3")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Follow latest" }));
    expect(screen.getByText("complete")).toBeInTheDocument();
    expect(screen.getByText("Following live Factory updates.")).toBeVisible();
  });
});

describe("hosted topology replay session restoration", () => {
  it("renders a quiet restored checkpoint and isolates retained sessions and generations", () => {
    const sessionA = identity("session-a");
    const sessionB = identity("session-b");
    const nextGeneration = identity("session-a", "generation-2");
    const timeline = useFactoryTimelineStore.getState();
    act(() => {
      timeline.appendEventForEntry(
        sessionA,
        topologyEvent({
          id: "session-a-initial",
          sequence: 1,
          stateName: "session-a-state",
          tick: 4,
          workTypeName: "story-a",
        }),
      );
    });
    const checkpoint =
      timeline.entryForIdentity(sessionA)?.currentReplayCheckpoint;
    if (!checkpoint) {
      throw new Error("expected session A checkpoint");
    }
    act(() => {
      timeline.resetEntry(sessionA);
      timeline.restoreCheckpointForEntry(sessionA, checkpoint);
      timeline.appendEventForEntry(
        sessionB,
        topologyEvent({
          id: "session-b-initial",
          sequence: 1,
          stateName: "session-b-state",
          tick: 7,
          workTypeName: "story-b",
        }),
      );
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: sessionA,
        streamState: liveStreamState,
      });
    });

    render(<HostedTopologyReplay />);
    expect(screen.getByText("session-a-state")).toBeInTheDocument();
    expect(screen.queryByText("session-b-state")).toBeNull();

    act(() => {
      useDashboardStreamStore.getState().setResolvedStreamIdentity(sessionB);
    });
    expect(screen.getByText("session-b-state")).toBeInTheDocument();
    expect(screen.queryByText("session-a-state")).toBeNull();

    act(() => {
      useDashboardStreamStore
        .getState()
        .setResolvedStreamIdentity(nextGeneration);
    });
    expect(screen.getByText("Loading Factory topology...")).toBeVisible();
    expect(screen.queryByText("session-a-state")).toBeNull();
    expect(screen.queryByText("session-b-state")).toBeNull();
  });
});

describe("hosted topology replay status and accessibility", () => {
  it("renders an intentional empty topology state with unavailable controls", () => {
    const session = identity("session-empty");
    const emptyEvent: FactoryEvent = {
      context: {
        eventTime: "2026-07-19T04:00:01.000Z",
        sequence: 1,
        tick: 1,
      },
      id: "empty-topology",
      payload: {
        factory: {
          name: "Empty factory",
          workers: [],
          workTypes: [],
          workstations: [],
        },
      },
      type: FACTORY_EVENT_TYPES.initialStructureRequest,
    };
    act(() => {
      useFactoryTimelineStore
        .getState()
        .appendEventForEntry(session, emptyEvent);
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: session,
        streamState: liveStreamState,
      });
    });

    render(<HostedTopologyReplay />);

    expect(
      screen.getByText("No Factory topology is available at this tick."),
    ).toBeVisible();
    expect(
      screen.getByRole("slider", { name: "Select Factory replay tick" }),
    ).toBeEnabled();
  });

  it("keeps the cached topology visible with localized reconnect status", () => {
    const session = identity("session-a");
    act(() => {
      useFactoryTimelineStore.getState().appendEventForEntry(
        session,
        topologyEvent({
          id: "initial",
          sequence: 1,
          stateName: "queued",
          tick: 1,
          workTypeName: "story",
        }),
      );
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: session,
        streamState: { message: "transport detail", status: "reconnecting" },
      });
    });

    render(<HostedTopologyReplay locale="zh-CN" />);

    expect(
      screen.getByText("正在重新连接工厂更新。正在显示最后收到的状态。"),
    ).toHaveAttribute("role", "status");
    expect(screen.getByText("queued")).toBeInTheDocument();
    expect(
      screen.getByRole("slider", { name: "选择工厂重放时刻" }),
    ).toHaveAttribute("aria-valuetext", "时刻 1");
    expect(screen.queryByText("transport detail")).toBeNull();
  });

  it("renders a contained transport failure without an inapplicable retry", () => {
    act(() => {
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: null,
        streamState: {
          message: "sensitive transport detail",
          status: "offline",
        },
      });
    });

    render(<HostedTopologyReplay />);

    const topologyFailure = screen.getByText(
      "The Factory topology could not be displayed.",
    ).parentElement;
    expect(topologyFailure).toHaveAttribute("role", "alert");
    expect(
      screen.getByText(
        "Factory updates are unavailable. Showing the last received state.",
      ),
    ).toHaveAttribute("role", "alert");
    expect(
      screen.getByRole("region", { name: "Factory topology" }),
    ).toContainElement(topologyFailure);
    expect(screen.queryByRole("button", { name: "Retry topology" })).toBeNull();
    expect(screen.queryByText("sensitive transport detail")).toBeNull();
    expect(
      screen.getByRole("slider", { name: "Select Factory replay tick" }),
    ).toBeDisabled();
  });
});

describe("hosted topology replay selection accessibility", () => {
  it("preserves hosted selection callbacks and selected-node evidence", () => {
    const session = identity("session-a");
    const onSelectStateNode = vi.fn();
    act(() => {
      useFactoryTimelineStore.getState().appendEventForEntry(
        session,
        topologyEvent({
          id: "initial",
          sequence: 1,
          stateName: "queued",
          tick: 1,
          workTypeName: "story",
        }),
      );
      useDashboardStreamStore.setState({
        resolvedStreamIdentity: session,
        streamState: liveStreamState,
      });
    });

    render(
      <HostedTopologyReplay
        onSelectStateNode={onSelectStateNode}
        selectedNodeID="work-state:story:queued"
      />,
    );
    const stateNode = screen.getByLabelText("Select queued work-state");
    expect(stateNode).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(stateNode);

    expect(onSelectStateNode).toHaveBeenCalledWith("story:queued");
  });
});
