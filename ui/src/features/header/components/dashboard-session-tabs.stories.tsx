import "../../../styles.css";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useLayoutEffect } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import { FACTORY_SESSIONS_QUERY_KEY } from "../../../api/factory-sessions/query-keys";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  DASHBOARD_REGRESSION_SESSION_IDS,
  dashboardRegressionAliasPlusUUIDSessions,
  dashboardRegressionSessionLists,
} from "../../../components/dashboard/fixtures";
import {
  historicalWorkOutcomeSnapshot,
  liveWorkOutcomeSnapshot,
} from "../../../stories/dashboardStorySupport";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { SESSION_TAB_PATH_MAX_LENGTH } from "../lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardSessionTabs } from "./dashboard-session-tabs";

const defaultSession = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: DASHBOARD_REGRESSION_SESSION_IDS.default,
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

export const DefaultSessionAbsolutePath = {
  parameters: defaultSessionAbsolutePathStoryParameters(),
  render: () => <SessionTabsStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const messages = getHeaderControlsMessages("en");
    const canvas = within(canvasElement);
    const absoluteFactoryPath =
      "/Users/operator/infinite-you/agent-factory/examples/catalog/factory";
    const truncatedSecondaryPath = `...${absoluteFactoryPath.slice(
      -(SESSION_TAB_PATH_MAX_LENGTH - 3),
    )}`;

    await expect(
      canvas.findByRole("navigation", { name: messages.sessionTabsLabel }),
    ).resolves.toBeVisible();
    await expect(canvas.getByRole("tab", { name: "factory" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(canvas.getByText(truncatedSecondaryPath)).toBeVisible();
    await expect(canvas.getByTitle(absoluteFactoryPath)).toHaveAttribute(
      "title",
      absoluteFactoryPath,
    );
  },
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
    const rootTab = canvas.getByRole("tab", { name: "root" });
    const betaTab = canvas.getByRole("tab", { name: "beta" });
    await expect(rootTab).toHaveAttribute("aria-selected", "true");
    await expect(betaTab).toBeVisible();
    const activeShellClassName = sessionTabShell(rootTab).className;
    const inactiveShellClassName = sessionTabShell(betaTab).className;
    expect(activeShellClassName).toContain("bg-surface-container-low");
    expect(activeShellClassName).not.toContain("border-outline-variant");
    expect(activeShellClassName).not.toContain("bg-surface-container-high");
    expect(activeShellClassName).not.toContain("bg-primary");
    expect(activeShellClassName).not.toContain("bg-primary-container");
    expect(inactiveShellClassName).toContain("text-on-surface-variant");
    expect(inactiveShellClassName).not.toContain("hover:border-outline");
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
      within(dialog).getByRole("button", {
        name: messages.openSessionSubmitLabel,
      }),
    );

    await userEvent.click(
      within(dialog).getByRole("button", { name: /review/i }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", {
        name: messages.openSessionTargetLabel,
      }),
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

export const CanonicalListReconciliation = {
  parameters: canonicalListReconciliationParameters(),
  render: () => <CanonicalListReconciliationStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const messages = getHeaderControlsMessages("en");
    const navigation = await canvas.findByRole("navigation", {
      name: messages.sessionTabsLabel,
    });

    await expect(navigation).toBeVisible();
    await expect(canvas.getAllByRole("tab")).toHaveLength(2);
    expect(canvasElement.textContent).not.toContain(DEFAULT_FACTORY_SESSION_ID);

    await userEvent.click(
      canvas.getByRole("button", { name: "Refresh sessions" }),
    );
    await waitFor(() => {
      expect(canvas.getByRole("tab", { name: "created" })).toHaveAttribute(
        "aria-selected",
        "false",
      );
    });
    expect(canvas.queryByRole("tab", { name: "secondary" })).toBeNull();
    expect(canvas.queryByRole("tab", { name: "removed" })).toBeNull();
    expect(canvasElement.textContent).not.toContain(DEFAULT_FACTORY_SESSION_ID);

    await userEvent.click(canvas.getByRole("button", { name: "Fail refresh" }));
    await expect(
      canvas.findByText(messages.sessionsErrorTitle),
    ).resolves.toBeVisible();
    await expect(canvas.getByRole("tab", { name: "created" })).toBeVisible();
    await userEvent.click(
      canvas.getByRole("button", { name: messages.retrySessionsLabel }),
    );
    await waitFor(() => {
      expect(canvas.queryByText(messages.sessionsErrorTitle)).toBeNull();
    });
  },
};

function sessionTabShell(tab: HTMLElement): HTMLElement {
  const shell = tab.parentElement;
  if (!shell) {
    throw new Error("Expected session tab shell wrapper");
  }
  return shell;
}

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

let canonicalListRequestCount = 0;
let failNextCanonicalListRefresh = false;

function CanonicalListReconciliationStory() {
  const queryClient = useQueryClient();

  useLayoutEffect(() => {
    canonicalListRequestCount = 0;
    failNextCanonicalListRefresh = false;
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  }, []);

  return (
    <div className="grid min-w-0 gap-3" style={{ maxWidth: "960px" }}>
      <button
        className="min-h-10 rounded-lg border border-outline px-3 py-2 text-sm"
        onClick={() => {
          void queryClient.invalidateQueries({
            queryKey: FACTORY_SESSIONS_QUERY_KEY,
          });
        }}
        type="button"
      >
        Refresh sessions
      </button>
      <button
        className="min-h-10 rounded-lg border border-outline px-3 py-2 text-sm"
        onClick={() => {
          failNextCanonicalListRefresh = true;
          void queryClient.invalidateQueries({
            queryKey: FACTORY_SESSIONS_QUERY_KEY,
          });
        }}
        type="button"
      >
        Fail refresh
      </button>
      <DashboardSessionTabs locale="en" />
    </div>
  );
}

function canonicalListReconciliationParameters() {
  return {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: () => {
            canonicalListRequestCount += 1;
            if (failNextCanonicalListRefresh) {
              failNextCanonicalListRefresh = false;
              return {
                body: {
                  code: "INTERNAL_ERROR",
                  message: "The fixture refresh failed.",
                },
                status: 503,
                statusText: "Service Unavailable",
              };
            }
            return {
              body: {
                sessions:
                  canonicalListRequestCount === 1
                    ? dashboardRegressionAliasPlusUUIDSessions
                    : dashboardRegressionSessionLists.refreshed,
              },
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

function defaultSessionAbsolutePathStoryParameters() {
  const absoluteFactoryPath =
    "/Users/operator/infinite-you/agent-factory/examples/catalog/factory";

  return {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: () => ({
            body: {
              sessions: [
                {
                  factoryDir: absoluteFactoryPath,
                  folderPath: absoluteFactoryPath,
                  id: DASHBOARD_REGRESSION_SESSION_IDS.default,
                  isDefault: true,
                  project: "factory",
                  target: {
                    kind: "default",
                  },
                },
              ],
            },
          }),
        },
      ],
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  };
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
                message:
                  "Unsupported folder for the Storybook session-tabs flow.",
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
