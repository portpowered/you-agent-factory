import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import { buildSuccessfulDurableSession } from "../../../testing/factory-session-event-replay-fixtures";
import {
  buildSuccessfulLiveProviderDispatchDetail,
  buildSuccessfulLiveProviderDispatchList,
  successfulLiveProviderDispatchID,
  successfulLiveProviderSessionID,
  successfulLiveProviderSessionRef,
} from "../../../testing/factory-session-live-provider-inspection-fixtures";
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

export const LiveProviderSuccessInspection = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${successfulLiveProviderSessionID}`,
          response: {
            body: buildSuccessfulDurableSession(successfulLiveProviderSessionID),
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
  render: () => renderFactorySessionDetailPanel(successfulLiveProviderSessionID),
};
