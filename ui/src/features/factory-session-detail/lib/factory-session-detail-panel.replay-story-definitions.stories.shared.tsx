import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import {
  awaitingReplaySessionID,
  buildAwaitingDurableSession,
  buildAwaitingReplayEventStream,
  buildSuccessfulDurableSession,
  buildSuccessfulReplayDispatchList,
  buildSuccessfulReplayEventStream,
  buildWarningDurableSession,
  buildWarningReplayDispatchList,
  buildWarningReplayEventStream,
  successfulReplaySessionID,
  unavailableReplaySessionID,
  warningReplaySessionID,
} from "../../../testing/factory-session-event-replay-fixtures";
import { FactorySessionDetailPanel } from "../components/factory-session-detail-panel";

function renderFactorySessionDetailPanel(sessionID: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return (
    <div style={{ maxWidth: "100%", width: "960px" }}>
      <QueryClientProvider client={queryClient}>
        <FactorySessionDetailPanel sessionID={sessionID} />
      </QueryClientProvider>
    </div>
  );
}

export const DurableReplayDisclosure = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}`,
          response: { body: buildSuccessfulDurableSession() },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}/events`,
          response: {
            body: buildSuccessfulReplayEventStream(),
            headers: { "Content-Type": "text/event-stream" },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}/dispatches`,
          response: { body: buildSuccessfulReplayDispatchList() },
        },
      ],
      sessionID: successfulReplaySessionID,
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText("Showing 5 Factory Events.");
    expect(canvas.getByText("Session completed")).toBeTruthy();
    expect(canvas.getByText("Dispatch status completed")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(successfulReplaySessionID),
};

export const DurableReplayDisclosureAwaitingApproval = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${awaitingReplaySessionID}`,
          response: { body: buildAwaitingDurableSession() },
        },
        {
          method: "GET",
          path: `/factory-sessions/${awaitingReplaySessionID}/events`,
          response: {
            body: buildAwaitingReplayEventStream(),
            headers: { "Content-Type": "text/event-stream" },
          },
        },
      ],
      sessionID: awaitingReplaySessionID,
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await user.click(trigger);
    await expect(canvas.findByText("Showing 2 Factory Events.")).resolves.toBeTruthy();
    await expect(canvas.getByText("Session result updated")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(awaitingReplaySessionID),
};

export const DurableReplayDisclosureWarning = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}`,
          response: { body: buildWarningDurableSession() },
        },
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}/events`,
          response: {
            body: buildWarningReplayEventStream(),
            headers: { "Content-Type": "text/event-stream" },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}/dispatches`,
          response: { body: buildWarningReplayDispatchList() },
        },
      ],
      sessionID: warningReplaySessionID,
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText("Checkpoint recorded");
    expect(canvas.getByText("Provider session timed out · Retry planned")).toBeTruthy();
    expect(canvas.getByText("Release verification failed.")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(warningReplaySessionID),
};

export const DurableReplayDisclosureUnavailable = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}`,
          response: { body: buildWarningDurableSession(unavailableReplaySessionID) },
        },
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}/events`,
          response: { status: 404 },
        },
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}/dispatches`,
          response: {
            body: { dispatches: [], sessionId: unavailableReplaySessionID },
          },
        },
      ],
      sessionID: unavailableReplaySessionID,
    },
  },
  render: () => renderFactorySessionDetailPanel(unavailableReplaySessionID),
};

export const SessionUnavailable = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions/session-missing",
          response: {
            body: {
              code: "NOT_FOUND",
              message: "Factory session not found.",
            },
            status: 404,
          },
        },
      ],
      sessionID: "session-missing",
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    expect(await canvas.findByText("This factory session is no longer available.")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel("session-missing"),
};

export const SessionError = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions/session-error",
          response: {
            body: {
              code: "INTERNAL_ERROR",
              message: "Factory session fetch failed.",
            },
            status: 500,
          },
        },
      ],
      sessionID: "session-error",
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    expect(await canvas.findByText("Factory session fetch failed.")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel("session-error"),
};
