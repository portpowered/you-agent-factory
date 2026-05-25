import "../../../styles.css";

import { useEffect } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  historicalWorkOutcomeSnapshot,
  liveWorkOutcomeSnapshot,
} from "../../../stories/dashboardStorySupport";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { DashboardSessionTabs } from "./dashboard-session-tabs";
import { getHeaderControlsMessages } from "../messages/header-controls";

const defaultSession = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: "~default",
  isDefault: true,
  project: "root",
  target: {
    kind: "default" as const,
  },
};

const betaSession = {
  factoryDir: "/workspace/root/beta",
  folderPath: "/workspace/root",
  id: "session-beta",
  isDefault: false,
  project: "beta",
  target: {
    kind: "named" as const,
    name: "beta",
  },
};

const reviewSession = {
  factoryDir: "/workspace/catalog/review",
  folderPath: "/workspace/catalog",
  id: "session-review",
  isDefault: false,
  project: "catalog",
  target: {
    kind: "named" as const,
    name: "review",
  },
};

export default {
  title: "you-agent-factory/Dashboard/Session Tabs",
  component: DashboardSessionTabs,
  tags: ["test"],
};

export const OpenFlowVerification = {
  parameters: sessionTabsStoryParameters(),
  render: () => <SessionTabsStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const messages = getHeaderControlsMessages("en");
    const canvas = within(canvasElement);
    const tabsNavigation = await canvas.findByRole("navigation", {
      name: messages.sessionTabsLabel,
    });
    const openButton = within(canvasElement.ownerDocument.body).getByRole(
      "button",
      { name: messages.openSessionButtonLabel },
    );

    await expect(tabsNavigation).toBeVisible();
    await expect(canvas.getByRole("tab", { name: "root" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(canvas.getByRole("tab", { name: "beta" })).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Close beta session" }),
    ).toBeVisible();

    await userEvent.click(openButton);

    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      { name: messages.openSessionDialogTitle },
    );
    const folderField = within(dialog).getByRole("textbox", {
      name: messages.sessionFolderFieldLabel,
    });
    await userEvent.clear(folderField);
    await userEvent.type(folderField, "/workspace/catalog");
    await userEvent.click(
      within(dialog).getByRole("button", { name: messages.openSessionSubmitLabel }),
    );

    await userEvent.click(
      within(dialog).getByRole("button", { name: /review/i }),
    );

    await waitFor(() => {
      expect(canvas.getByRole("tab", { name: "review" })).toHaveAttribute(
        "aria-selected",
        "true",
      );
    });
    await userEvent.click(
      canvas.getByRole("button", { name: "Close review session" }),
    );
    await waitFor(() => {
      expect(canvas.getByRole("tab", { name: "root" })).toHaveAttribute(
        "aria-selected",
        "true",
      );
    });
    await expect(canvas.queryByRole("tab", { name: "review" })).toBeNull();
  },
};

function SessionTabsStory() {
  useEffect(() => {
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  }, []);

  return (
    <div style={{ maxWidth: "100%", width: "960px" }}>
      <DashboardSessionTabs locale="en" />
    </div>
  );
}

function sessionTabsStoryParameters() {
  let defaultSessionClosed = false;
  let openedReviewSession = false;
  let reviewSessionClosed = false;

  return {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: () => ({
            body: {
              sessions: openedReviewSession
                ? reviewSessionClosed
                  ? defaultSessionClosed
                    ? [betaSession]
                    : [defaultSession, betaSession]
                  : defaultSessionClosed
                    ? [betaSession, reviewSession]
                    : [defaultSession, betaSession, reviewSession]
                : defaultSessionClosed
                  ? [betaSession]
                  : [defaultSession, betaSession],
            },
          }),
        },
        {
          method: "POST",
          path: "/factory-sessions",
          response: async (_input: RequestInfo | URL, init?: RequestInit) => {
            const body =
              typeof init?.body === "string"
                ? (JSON.parse(init.body) as {
                    folderPath?: string;
                    validateOnly?: boolean;
                    target?: { kind?: string; name?: string };
                  })
                : {};

            if (
              body.folderPath === "/workspace/catalog" &&
              body.validateOnly === true
            ) {
              return {
                body: {
                  targets: [
                    {
                      factoryDir: "/workspace/catalog/review",
                      folderPath: "/workspace/catalog",
                      label: "Catalog / review",
                      project: "catalog",
                      ref: {
                        kind: "named",
                        name: "review",
                      },
                    },
                    {
                      factoryDir: "/workspace/catalog/plan",
                      folderPath: "/workspace/catalog",
                      label: "Catalog / plan",
                      project: "catalog",
                      ref: {
                        kind: "named",
                        name: "plan",
                      },
                    },
                  ],
                },
              };
            }

            if (
              body.folderPath === "/workspace/catalog" &&
              body.target?.kind === "named" &&
              body.target.name === "review"
            ) {
              openedReviewSession = true;
              return {
                body: {
                  session: reviewSession,
                },
                status: 201,
              };
            }
            return {
              body: {
                code: "BAD_REQUEST",
                message: "Unsupported folder for the Storybook session-tabs flow.",
              },
              status: 400,
              statusText: "Bad Request",
            };
          },
        },
        {
          method: "DELETE",
          path: "/factory-sessions/session-review",
          response: () => {
            reviewSessionClosed = true;
            return {
              status: 204,
            };
          },
        },
        {
          method: "DELETE",
          path: "/factory-sessions/~default",
          response: () => {
            defaultSessionClosed = true;
            return {
              status: 204,
            };
          },
        },
      ],
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  };
}
