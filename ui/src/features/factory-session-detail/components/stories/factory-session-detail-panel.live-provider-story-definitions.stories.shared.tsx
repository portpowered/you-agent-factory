import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import {
  buildSuccessfulDurableSession,
  buildSuccessfulReplayEventStream,
} from "../../../../testing/factory-session-event-replay-fixtures";
import {
  buildFailedBridgedChildDispatchDetail,
  buildFailedBridgedChildDispatchList,
  buildFailedBridgedChildDurableSession,
  buildSuccessfulLiveProviderDispatchDetail,
  buildSuccessfulLiveProviderDispatchList,
  failedBridgedChildDispatchID,
  failedBridgedChildProviderSessionRef,
  failedBridgedChildSessionID,
  successfulLiveProviderDispatchID,
  successfulLiveProviderSessionID,
  successfulLiveProviderSessionRef,
} from "../../../../testing/factory-session-live-provider-inspection-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";

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

export const LiveProviderSuccessInspection = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}`,
          response: {
            body: buildSuccessfulDurableSession(
              successfulLiveProviderSessionID,
            ),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/dispatches`,
          response: {
            body: buildSuccessfulLiveProviderDispatchList(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/dispatches/${successfulLiveProviderDispatchID}`,
          response: {
            body: buildSuccessfulLiveProviderDispatchDetail(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=final`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=partial`,
          response: {
            status: 404,
          },
        },
      ],
      sessionID: successfulLiveProviderSessionID,
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    expect(await canvas.findByText("Execution mode: live")).toBeTruthy();
    expect(
      await canvas.findByText(
        `Provider session: ${successfulLiveProviderSessionRef.provider} / ${successfulLiveProviderSessionRef.kind} / ${successfulLiveProviderSessionRef.id}`,
      ),
    ).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
      }),
    );

    expect(await canvas.findByText("JavaScript task")).toBeTruthy();
    expect(await canvas.findByText("Provider sessions")).toBeTruthy();
    expect(
      await canvas.findByText(
        `${successfulLiveProviderSessionRef.kind} · ${successfulLiveProviderSessionRef.id}`,
      ),
    ).toBeTruthy();
    expect(
      await canvas.findByRole("link", { name: "art-js-success-001" }),
    ).toBeTruthy();
  },
  render: () =>
    renderFactorySessionDetailPanel(successfulLiveProviderSessionID),
};

export const FailedBridgedChildInspection = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${failedBridgedChildSessionID}`,
          response: {
            body: buildFailedBridgedChildDurableSession(
              failedBridgedChildSessionID,
            ),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${failedBridgedChildSessionID}/dispatches`,
          response: {
            body: buildFailedBridgedChildDispatchList(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${failedBridgedChildSessionID}/dispatches/${failedBridgedChildDispatchID}`,
          response: {
            body: buildFailedBridgedChildDispatchDetail(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${failedBridgedChildSessionID}/results?mode=final`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${failedBridgedChildSessionID}/results?mode=partial`,
          response: {
            status: 404,
          },
        },
      ],
      sessionID: failedBridgedChildSessionID,
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    expect(await canvas.findByText("Execution mode: live")).toBeTruthy();
    expect(
      await canvas.findByText(
        `Provider session: ${failedBridgedChildProviderSessionRef.provider} / ${failedBridgedChildProviderSessionRef.kind} / ${failedBridgedChildProviderSessionRef.id}`,
      ),
    ).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: `Expand dispatch detail for ${failedBridgedChildDispatchID}`,
      }),
    );

    expect(await canvas.findByText("Failure detail")).toBeTruthy();
    expect(await canvas.findByText("VERIFY_ASSERTION_FAILED")).toBeTruthy();
    expect(await canvas.findByText("verification_error")).toBeTruthy();
    expect(
      await canvas.findByText("Expected release manifest checksum."),
    ).toBeTruthy();
    expect(await canvas.findByText("JavaScript task")).toBeTruthy();
    expect(await canvas.findByText("Provider sessions")).toBeTruthy();
    expect(
      await canvas.findByText(
        `${failedBridgedChildProviderSessionRef.kind} · ${failedBridgedChildProviderSessionRef.id}`,
      ),
    ).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(failedBridgedChildSessionID),
};

export const LiveProviderAdjacentSurfacesRegression = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}`,
          response: {
            body: buildSuccessfulDurableSession(
              successfulLiveProviderSessionID,
            ),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/dispatches`,
          response: {
            body: buildSuccessfulLiveProviderDispatchList(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/dispatches/${successfulLiveProviderDispatchID}`,
          response: {
            body: buildSuccessfulLiveProviderDispatchDetail(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/events`,
          response: {
            body: buildSuccessfulReplayEventStream(
              successfulLiveProviderSessionID,
            ),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=final`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=partial`,
          response: {
            status: 404,
          },
        },
      ],
      sessionID: successfulLiveProviderSessionID,
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    expect(
      await canvas.findByText("art-js-success-001 · FINAL_RESULT"),
    ).toBeTruthy();
    expect(await canvas.findByText("Execution mode: live")).toBeTruthy();
    expect(
      await canvas.findByText(
        `Provider session: ${successfulLiveProviderSessionRef.provider} / ${successfulLiveProviderSessionRef.kind} / ${successfulLiveProviderSessionRef.id}`,
      ),
    ).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
      }),
    );
    expect(
      await canvas.findByRole("link", { name: "art-js-success-001" }),
    ).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Expand Factory Event replay",
      }),
    );
    expect(await canvas.findByText("Showing 5 Factory Events.")).toBeTruthy();
    expect(await canvas.findByText("Session started")).toBeTruthy();
    expect(
      await canvas.findByText(
        `Provider session: ${successfulLiveProviderSessionRef.provider} / ${successfulLiveProviderSessionRef.kind} / ${successfulLiveProviderSessionRef.id}`,
      ),
    ).toBeTruthy();
  },
  render: () =>
    renderFactorySessionDetailPanel(successfulLiveProviderSessionID),
};

export const LiveProviderDispatchDetailUnavailable = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}`,
          response: {
            body: buildSuccessfulDurableSession(
              successfulLiveProviderSessionID,
            ),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/dispatches`,
          response: {
            body: buildSuccessfulLiveProviderDispatchList(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/dispatches/${successfulLiveProviderDispatchID}`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=final`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=partial`,
          response: {
            status: 404,
          },
        },
      ],
      sessionID: successfulLiveProviderSessionID,
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    expect(await canvas.findByText("Execution mode: live")).toBeTruthy();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
      }),
    );
    expect(
      await canvas.findByText(
        `Dispatch detail for ${successfulLiveProviderDispatchID} is no longer available.`,
      ),
    ).toBeTruthy();
    expect(
      await canvas.findByText(
        `Provider session: ${successfulLiveProviderSessionRef.provider} / ${successfulLiveProviderSessionRef.kind} / ${successfulLiveProviderSessionRef.id}`,
      ),
    ).toBeTruthy();
    expect(
      await canvas.findByRole("button", {
        name: "Expand Factory Event replay",
      }),
    ).toBeTruthy();
  },
  render: () =>
    renderFactorySessionDetailPanel(successfulLiveProviderSessionID),
};
