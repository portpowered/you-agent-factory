import { expect, waitFor, within } from "storybook/test";

import "../../../styles.css";
import { ProviderSessionWidget } from "../../provider-session-detail/public";
import { DASHBOARD_WIDGET_IDS } from "../hooks/dashboardLayoutSchema";
import {
  expectBentoHeaderDragSurface,
  layoutFor,
  populatedProviderSession,
  providerSessionEmptyFetchMock,
  providerSessionEmptyID,
  providerSessionErrorFetchMock,
  providerSessionErrorID,
  providerSessionFetchMock,
  providerSessionLoadingFetchMock,
  providerSessionLoadingID,
  renderCardFrame,
  renderProviderSessionStateCard,
  semanticWorkflowDashboardSnapshot,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const ProviderSession = {
  parameters: {
    dashboardApi: {
      fetchMocks: [providerSessionFetchMock],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () =>
    renderCardFrame({
      children: (
        <ProviderSessionWidget
          selectedProviderSession={populatedProviderSession}
          widgetId="provider-session::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.providerSession, {
        h: 6,
        id: "provider-session::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(await within(card).findByText("Transcript")).toBeVisible();
    expectBentoHeaderDragSurface(card, "Provider session");
  },
};

export const ProviderSessionLoading = {
  ...renderProviderSessionStateCard({
    fetchMock: providerSessionLoadingFetchMock,
    sessionID: providerSessionLoadingID,
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(await within(card).findByRole("status")).toHaveTextContent(
      "Loading session details...",
    );
  },
};

export const ProviderSessionEmpty = {
  render: () =>
    renderCardFrame({
      children: (
        <ProviderSessionWidget
          selectedProviderSession={null}
          widgetId="provider-session-empty::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.providerSession, {
        h: 6,
        id: "provider-session-empty::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(
      within(card).getByText(
        "Select a provider session from work-item or workstation history to inspect session details.",
      ),
    ).toBeVisible();
  },
};

export const ProviderSessionEmptyFile = {
  ...renderProviderSessionStateCard({
    fetchMock: providerSessionEmptyFetchMock,
    sessionID: providerSessionEmptyID,
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await waitFor(() => {
      expect(within(card).getByRole("status")).toHaveTextContent(
        "The selected session file did not contain any Codex event records.",
      );
    });
  },
};

export const ProviderSessionError = {
  ...renderProviderSessionStateCard({
    fetchMock: providerSessionErrorFetchMock,
    sessionID: providerSessionErrorID,
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(await within(card).findByRole("alert")).toHaveTextContent(
      "Storybook provider-session failure",
    );
  },
};
