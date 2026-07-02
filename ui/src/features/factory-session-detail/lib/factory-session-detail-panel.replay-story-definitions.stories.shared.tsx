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

function renderFactorySessionDetailPanel(sessionID: string, width = "960px") {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return (
    <div style={{ maxWidth: "100%", width }}>
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
    dashboardApi: (() => {
      let approved = false;

      return {
        fetchMocks: [
          {
            method: "GET",
            path: `/factory-sessions/${awaitingReplaySessionID}`,
            response: () => ({
              body: approved
                ? {
                    ...buildAwaitingDurableSession(),
                    lifecycle: {
                      startedAt: "2026-06-08T15:00:05Z",
                      updatedAt: "2026-06-08T15:00:05Z",
                    },
                    progress: {
                      completedDispatches: 0,
                      failedDispatches: 0,
                      inFlightDispatches: 1,
                      totalDispatches: 1,
                    },
                    resultSummary: {
                      resultStatus: "NOT_READY",
                      summary: "Execution resumed after approval.",
                    },
                    status: "RUNNING",
                  }
                : buildAwaitingDurableSession(),
            }),
          },
          {
            method: "POST",
            path: `/factory-sessions/${awaitingReplaySessionID}/approve`,
            response: () => {
              approved = true;

              return {
                body: {
                  detail: "Approval request was accepted.",
                  operation: "APPROVE",
                  outcome: "ACCEPTED",
                  sessionId: awaitingReplaySessionID,
                  session: {
                    ...buildAwaitingDurableSession(),
                    lifecycle: {
                      startedAt: "2026-06-08T15:00:05Z",
                      updatedAt: "2026-06-08T15:00:05Z",
                    },
                    progress: {
                      completedDispatches: 0,
                      failedDispatches: 0,
                      inFlightDispatches: 1,
                      totalDispatches: 1,
                    },
                    resultSummary: {
                      resultStatus: "NOT_READY",
                      summary: "Execution resumed after approval.",
                    },
                    status: "RUNNING",
                  },
                  status: "RUNNING",
                },
                status: 202,
              };
            },
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
      };
    })(),
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(await canvas.findByRole("button", { name: "Approve" }));
    await expect(canvas.findByText("Accepted")).resolves.toBeTruthy();
    await expect(canvas.findByText("Approve accepted")).resolves.toBeTruthy();
    await expect(
      canvas.findByText(
        "Approval request was accepted. Current durable status: Running.",
      ),
    ).resolves.toBeTruthy();
    await expect(canvas.findByRole("button", { name: "Pause" })).resolves.toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(awaitingReplaySessionID),
};

export const DurableReplayDisclosureAwaitingApprovalMobile = {
  tags: ["test"],
  parameters: {
    ...DurableReplayDisclosureAwaitingApproval.parameters,
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await expect(canvas.findByText("Lifecycle controls")).resolves.toBeTruthy();
    await expect(canvas.findByRole("button", { name: "Approve" })).resolves.toBeTruthy();
    await expect(canvas.findByText("Runtime")).resolves.toBeTruthy();
    await user.click(await canvas.findByRole("button", { name: "Approve" }));
    await expect(canvas.findByText("Accepted")).resolves.toBeTruthy();
    await expect(canvas.findByRole("button", { name: "Pause" })).resolves.toBeTruthy();
    await expect(
      canvas.findByRole("button", { name: "Expand Factory Event replay" }),
    ).resolves.toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(awaitingReplaySessionID, "375px"),
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
