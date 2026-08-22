import { useQueryClient } from "@tanstack/react-query";
import { useLayoutEffect, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import type { DashboardTrace } from "../../../api/dashboard/types";
import {
  factoryTimelineEntryKey,
  useFactoryTimelineStore,
} from "../../timeline/public/store";
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";
import { TraceGridBentoCard } from "../components/trace-grid-card";
import { dashboardTraceQueryKey } from "./useTrace";
import { useTraceDrilldown } from "./useTraceDrilldown";

const STREAM_IDENTITY_A: StreamDerivedCacheIdentity = {
  backendScopeID: "storybook-backend",
  factorySessionID: "storybook-session-a",
  logicalSessionKeyID: "storybook-logical-a",
  streamGenerationID: "storybook-generation-a",
};

const STREAM_IDENTITY_B: StreamDerivedCacheIdentity = {
  backendScopeID: "storybook-backend",
  factorySessionID: "storybook-session-b",
  logicalSessionKeyID: "storybook-logical-b",
  streamGenerationID: "storybook-generation-b",
};

const SHARED_WORK_ID = "storybook-shared-work";

function buildTrace(traceID: string): DashboardTrace {
  return {
    dispatches: [
      {
        dispatch_id: `${traceID}-dispatch`,
        end_time: "2026-08-15T12:00:01Z",
        outcome: "ACCEPTED",
        start_time: "2026-08-15T12:00:00Z",
        transition_id: "review",
        workstation_name: "Review",
      },
    ],
    relations: [],
    request_ids: [],
    trace_id: traceID,
    transition_ids: [],
    work_ids: [SHARED_WORK_ID],
    work_items: [],
    workstation_sequence: [],
  };
}

function seedTrace(
  streamIdentity: StreamDerivedCacheIdentity,
  trace: DashboardTrace,
): void {
  const store = useFactoryTimelineStore.getState();
  const entryKey = factoryTimelineEntryKey(streamIdentity);
  if (!store.entriesByKey[entryKey]) {
    store.activateEntry(streamIdentity);
  }

  useFactoryTimelineStore.setState((state) => {
    const entry = state.entriesByKey[entryKey];
    if (!entry) {
      throw new Error("Expected the Storybook trace timeline entry.");
    }
    const snapshot = entry.worldViewCache[0];
    if (!snapshot) {
      throw new Error("Expected the Storybook trace timeline snapshot.");
    }

    const nextEntry = {
      ...entry,
      selectedTick: 0,
      worldViewCache: {
        0: {
          ...snapshot,
          tracesByWorkID: { [SHARED_WORK_ID]: trace },
        },
      },
    };

    return {
      ...(state.activeEntryKey === entryKey ? nextEntry : {}),
      entriesByKey: {
        ...state.entriesByKey,
        [entryKey]: nextEntry,
      },
    };
  });
}

function TraceSessionScopeStory() {
  const queryClient = useQueryClient();
  const [selectedIdentity, setSelectedIdentity] = useState(STREAM_IDENTITY_A);
  const { traceGridState } = useTraceDrilldown(
    SHARED_WORK_ID,
    null,
    "en",
    selectedIdentity,
  );

  useLayoutEffect(() => {
    useFactoryTimelineStore.getState().reset();
    seedTrace(STREAM_IDENTITY_A, buildTrace("trace-session-a"));
    seedTrace(STREAM_IDENTITY_B, buildTrace("trace-session-b"));
    useFactoryTimelineStore.getState().activateEntry(STREAM_IDENTITY_A);
  }, []);

  function deliverLateSessionAResult() {
    const lateTrace = buildTrace("trace-session-a-late");
    seedTrace(STREAM_IDENTITY_A, lateTrace);
    queryClient.setQueryData(
      dashboardTraceQueryKey(SHARED_WORK_ID, null, 0, STREAM_IDENTITY_A),
      lateTrace,
    );
  }

  return (
    <main className="grid min-h-screen gap-4 bg-surface p-4 text-on-surface">
      <div className="flex flex-wrap items-center gap-2">
        <span aria-live="polite" role="status">
          Selected stream: {selectedIdentity === STREAM_IDENTITY_A ? "A" : "B"}
        </span>
        <button
          onClick={() => setSelectedIdentity(STREAM_IDENTITY_A)}
          type="button"
        >
          Select session A
        </button>
        <button
          onClick={() => setSelectedIdentity(STREAM_IDENTITY_B)}
          type="button"
        >
          Select session B
        </button>
        <button onClick={deliverLateSessionAResult} type="button">
          Deliver late session A result
        </button>
      </div>
      <TraceGridBentoCard state={traceGridState} />
    </main>
  );
}

export default {
  title: "you-agent-factory/Trace Drilldown/Session Scope",
  component: TraceSessionScopeStory,
  tags: ["test"],
};

export const LateResultDoesNotCrossSessions = {
  render: () => <TraceSessionScopeStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(
      await within(card).findByText("trace-session-a"),
    ).toBeVisible();
    await userEvent.click(
      canvas.getByRole("button", { name: "Select session B" }),
    );
    await waitFor(() => {
      expect(within(card).getByText("trace-session-b")).toBeVisible();
    });

    await userEvent.click(
      canvas.getByRole("button", {
        name: "Deliver late session A result",
      }),
    );
    await waitFor(() => {
      expect(within(card).getByText("trace-session-b")).toBeVisible();
      expect(within(card).queryByText("trace-session-a-late")).toBeNull();
    });
  },
};
