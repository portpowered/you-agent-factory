import { expect, userEvent, within } from "storybook/test";

import {
  awaitingReplaySessionID,
  buildAwaitingDurableSession,
  buildAwaitingReplayEventStream,
  buildEmptyReplayEventStream,
  buildSuccessfulDurableSession,
  buildSuccessfulReplayDispatchList,
  buildSuccessfulReplayEventStream,
  buildWarningDurableSession,
  buildWarningReplayDispatchList,
  buildWarningReplayEventStream,
  emptyReplaySessionID,
  errorReplaySessionID,
  successfulReplaySessionID,
  unavailableReplaySessionID,
  warningReplaySessionID,
} from "../../../../testing/factory-session-event-replay-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import { renderFactorySessionDetailPanel } from "../factory-session-detail-panel.stories.shared";

export default {
  title:
    "you-agent-factory/Current Selection/Factory Session Detail Panel/Event Replay",
  component: FactorySessionDetailPanel,
};

export const DurableReplayDisclosure = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}`,
          response: {
            body: buildSuccessfulDurableSession(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}/events`,
          response: {
            body: buildSuccessfulReplayEventStream(),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${successfulReplaySessionID}/dispatches`,
          response: {
            body: buildSuccessfulReplayDispatchList(),
          },
        },
      ],
      sessionID: successfulReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText("Showing 5 Factory Events.");
    expect(canvas.getByText("Session completed")).toBeTruthy();
    expect(canvas.getByText("Dispatch status completed")).toBeTruthy();

    const timeline = canvas
      .getAllByRole("list")
      .find((element) => element.tagName === "OL");
    if (!timeline) {
      throw new Error("Expected replay timeline <ol> to be present.");
    }
    const timelineItems = within(timeline).getAllByRole("listitem");
    expect(timelineItems).toHaveLength(5);
    const firstItem = timelineItems[0];
    const lastItem = timelineItems[4];
    if (!firstItem || !lastItem) {
      throw new Error("Expected replay timeline to include five list items.");
    }
    expect(within(firstItem).getByText("Session started")).toBeTruthy();
    expect(within(lastItem).getByText("Session event 5 · Tick 5")).toBeTruthy();
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
          response: {
            body: buildAwaitingDurableSession(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${awaitingReplaySessionID}/events`,
          response: {
            body: buildAwaitingReplayEventStream(),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
      ],
      sessionID: awaitingReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();

    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await user.click(trigger);
    await expect(
      canvas.findByText("Showing 2 Factory Events."),
    ).resolves.toBeTruthy();
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
          response: {
            body: buildWarningDurableSession(),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}/events`,
          response: {
            body: buildWarningReplayEventStream(),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${warningReplaySessionID}/dispatches`,
          response: {
            body: buildWarningReplayDispatchList(),
          },
        },
      ],
      sessionID: warningReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText("Checkpoint recorded");
    expect(
      canvas.getByText("Provider session timed out · Retry planned"),
    ).toBeTruthy();
    expect(canvas.getByText("Release verification failed.")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(warningReplaySessionID),
};

export const DurableReplayDisclosureEmpty = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${emptyReplaySessionID}`,
          response: {
            body: buildSuccessfulDurableSession(emptyReplaySessionID),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${emptyReplaySessionID}/events`,
          response: {
            body: buildEmptyReplayEventStream(),
            headers: {
              "Content-Type": "text/event-stream",
            },
          },
        },
      ],
      sessionID: emptyReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText(
      "No durable Factory Events are available for this session.",
    );
    expect(
      canvas.queryAllByRole("list").some((element) => element.tagName === "OL"),
    ).toBe(false);
  },
  render: () => renderFactorySessionDetailPanel(emptyReplaySessionID),
};

export const DurableReplayDisclosureUnavailable = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}`,
          response: {
            body: buildWarningDurableSession(unavailableReplaySessionID),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}/events`,
          response: {
            status: 404,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${unavailableReplaySessionID}/dispatches`,
          response: {
            body: {
              dispatches: [],
              sessionId: unavailableReplaySessionID,
            },
          },
        },
      ],
      sessionID: unavailableReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText(
      "Durable Factory Event replay is unavailable for this session.",
    );
    expect(canvas.getByText("Partial result ref")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(unavailableReplaySessionID),
};

export const DurableReplayDisclosureError = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/factory-sessions/${errorReplaySessionID}`,
          response: {
            body: buildWarningDurableSession(errorReplaySessionID),
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${errorReplaySessionID}/events`,
          response: {
            body: {
              code: "INTERNAL_ERROR",
              message: "replay boom",
            },
            status: 500,
          },
        },
        {
          method: "GET",
          path: `/factory-sessions/${errorReplaySessionID}/dispatches`,
          response: {
            body: buildWarningReplayDispatchList(errorReplaySessionID),
          },
        },
      ],
      sessionID: errorReplaySessionID,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", {
      name: "Expand Factory Event replay",
    });
    await userEvent.click(trigger);
    await canvas.findByText("replay boom");
    expect(canvas.getByText("Artifacts")).toBeTruthy();
  },
  render: () => renderFactorySessionDetailPanel(errorReplaySessionID),
};
