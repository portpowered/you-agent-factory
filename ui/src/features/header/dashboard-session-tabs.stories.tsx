import "../../styles.css";

import { useEffect } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import { DEFAULT_FACTORY_SESSION_ID } from "../../api/session-routing";
import {
  historicalWorkOutcomeSnapshot,
  liveWorkOutcomeSnapshot,
} from "../../stories/dashboardStorySupport";
import { useDashboardSessionStore } from "../dashboard/state/dashboardSessionStore";
import { DashboardSessionTabs } from "./dashboard-session-tabs";

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
  title: "Infinite You/Dashboard/Session Tabs",
  component: DashboardSessionTabs,
  tags: ["test"],
};

export const OpenFlowVerification = {
  parameters: sessionTabsStoryParameters(),
  render: () => <SessionTabsStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const tabsNavigation = await canvas.findByRole("navigation", {
      name: "factory sessions",
    });
    const openButton = within(canvasElement.ownerDocument.body).getByRole(
      "button",
      { name: "Open another session" },
    );

    await expect(tabsNavigation).toBeVisible();
    await expect(canvas.getByRole("tab", { name: /root \/ default/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(canvas.getByRole("tab", { name: /root \/ beta/i })).toBeVisible();

    await userEvent.click(openButton);

    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      { name: "Open factory session" },
    );
    const folderField = within(dialog).getByRole("textbox", {
      name: "Factory folder",
    });
    await userEvent.clear(folderField);
    await userEvent.type(folderField, "/workspace/catalog");
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Inspect folder" }),
    );

    await expect(
      await within(dialog).findByRole("region", { name: "Pick a runnable target" }),
    ).toBeVisible();
    await expect(within(dialog).getByText("Choose one runnable target from this folder."))
      .toBeVisible();
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Catalog / review catalog" }),
    );

    await waitFor(() => {
      expect(
        canvas.getByRole("tab", { name: /catalog \/ review/i }),
      ).toHaveAttribute("aria-selected", "true");
    });
    await expect(canvas.getByText("Active folder: /workspace/catalog")).toBeVisible();

    await userEvent.click(
      canvas.getByRole("button", { name: "Close catalog / review session" }),
    );
    await waitFor(() => {
      expect(
        canvas.getByRole("tab", { name: /root \/ default/i }),
      ).toHaveAttribute("aria-selected", "true");
    });
    await expect(
      canvas.queryByRole("tab", { name: /catalog \/ review/i }),
    ).toBeNull();
    await expect(canvas.getByText("Active folder: /workspace/root")).toBeVisible();
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
                    target?: { kind?: string; name?: string };
                  })
                : {};

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

            if (body.folderPath === "/workspace/catalog") {
              return {
                body: {
                  targets: [
                    {
                      factoryDir: "/workspace/catalog/review",
                      label: "Catalog / review",
                      project: "catalog",
                      ref: {
                        kind: "named",
                        name: "review",
                      },
                    },
                    {
                      factoryDir: "/workspace/catalog/plan",
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
