// biome-ignore-all lint/style/noExcessiveLinesPerFile: existing dashboard-session-tabs coverage stayed intact during feature-root migration.
// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: existing dashboard-session-tabs coverage stayed intact during feature-root migration.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { FactorySessionsAPIError } from "../../../api/factory-sessions";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  factorySessionFieldTarget,
  factorySessionTargetTarget,
} from "../../../testing/factory-validation-target-fixtures";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import {
  SESSION_TAB_PATH_MAX_LENGTH,
  sessionCloseLabel,
  sessionTabSecondaryPath,
} from "../lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardSessionTabs } from "./dashboard-session-tabs";

const listFactorySessions = vi.fn();
const openFactorySession = vi.fn();
const closeFactorySession = vi.fn();

vi.mock("../../../api/factory-sessions", () => ({
  FactorySessionsAPIError: class FactorySessionsAPIError extends Error {
    public readonly code: string;
    public readonly responseBody?: unknown;
    public readonly status?: number;
    public readonly statusText?: string;
    public readonly targets?: unknown[];

    public constructor(
      message: string,
      details: {
        code: string;
        responseBody?: unknown;
        status?: number;
        statusText?: string;
        targets?: unknown[];
      },
    ) {
      super(message);
      this.name = "FactorySessionsAPIError";
      this.code = details.code;
      this.responseBody = details.responseBody;
      this.status = details.status;
      this.statusText = details.statusText;
      this.targets = details.targets;
    }
  },
  listFactorySessions: (...args: unknown[]) => listFactorySessions(...args),
  closeFactorySession: (...args: unknown[]) => closeFactorySession(...args),
  openFactorySession: (...args: unknown[]) => openFactorySession(...args),
}));

describe("DashboardSessionTabs", () => {
  beforeEach(() => {
    listFactorySessions.mockReset();
    openFactorySession.mockReset();
    closeFactorySession.mockReset();
    vi.unstubAllGlobals();
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  it("renders loading and then the active session tabs", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        target: {
          kind: "named",
          name: "beta",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    expect(screen.getByText(messages.loadingSessionsLabel)).toBeTruthy();

    await waitFor(() => {
      expect(
        screen.getByRole("navigation", { name: messages.sessionTabsLabel }),
      ).toBeTruthy();
    });

    expect(screen.getByRole("tablist")).toBeTruthy();
    expect(screen.getByRole("tab", { name: "root" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "beta" })).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: messages.openSessionButtonLabel })
        .closest('[role="tablist"]'),
    ).toBeNull();
    expect(screen.getByRole("tabpanel")).toBeTruthy();
  });

  it("uses subtle active styling on the session tab shell without accent fill", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        target: {
          kind: "named",
          name: "beta",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("navigation", { name: messages.sessionTabsLabel }),
      ).toBeTruthy();
    });

    const rootTab = screen.getByRole("tab", { name: "root" });
    const betaTab = screen.getByRole("tab", { name: "beta" });
    const activeShell = sessionTabShell(rootTab);
    const inactiveShell = sessionTabShell(betaTab);

    expectSubtleActiveSessionTabShell(activeShell);
    expectMutedInactiveSessionTabShell(inactiveShell);

    fireEvent.click(betaTab);

    await waitFor(() => {
      expect(betaTab.getAttribute("aria-selected")).toBe("true");
    });

    expectSubtleActiveSessionTabShell(sessionTabShell(betaTab));
    expectMutedInactiveSessionTabShell(sessionTabShell(rootTab));
  });

  it("supports keyboard navigation across session tabs with roving tab focus", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        target: {
          kind: "named",
          name: "beta",
        },
      },
      {
        factoryDir: "/workspace/root/gamma",
        folderPath: "/workspace/root",
        id: "session-gamma",
        isDefault: false,
        project: "gamma",
        target: {
          kind: "named",
          name: "gamma",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("navigation", { name: messages.sessionTabsLabel }),
      ).toBeTruthy();
    });

    const rootTab = screen.getByRole("tab", { name: "root" });
    const betaTab = screen.getByRole("tab", { name: "beta" });
    const gammaTab = screen.getByRole("tab", { name: "gamma" });

    expect(rootTab.getAttribute("aria-selected")).toBe("true");
    expect(rootTab.getAttribute("tabindex")).toBe("0");
    expect(betaTab.getAttribute("tabindex")).toBe("-1");
    rootTab.focus();

    fireEvent.keyDown(rootTab, { key: "ArrowRight" });
    await waitFor(() => {
      expect(betaTab.getAttribute("aria-selected")).toBe("true");
    });
    expect(document.activeElement).toBe(betaTab);
    expect(betaTab.getAttribute("tabindex")).toBe("0");
    expect(rootTab.getAttribute("tabindex")).toBe("-1");

    fireEvent.keyDown(betaTab, { key: "End" });
    await waitFor(() => {
      expect(gammaTab.getAttribute("aria-selected")).toBe("true");
    });
    expect(document.activeElement).toBe(gammaTab);
    expect(gammaTab.getAttribute("tabindex")).toBe("0");

    fireEvent.keyDown(gammaTab, { key: "Home" });
    await waitFor(() => {
      expect(rootTab.getAttribute("aria-selected")).toBe("true");
    });
    expect(document.activeElement).toBe(rootTab);
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "~default",
    );
  });

  it("keeps overflowed session tabs reachable through a horizontal tab strip", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: "/workspace/root/alpha",
        folderPath: "/workspace/root/alpha",
        id: "session-alpha",
        isDefault: false,
        project: "alpha",
        target: {
          kind: "named",
          name: "alpha",
        },
      },
      {
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root/beta",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        target: {
          kind: "named",
          name: "beta",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("navigation", { name: messages.sessionTabsLabel }),
      ).toBeTruthy();
    });

    const navigation = screen.getByRole("navigation", {
      name: messages.sessionTabsLabel,
    });
    const tablist = screen.getByRole("tablist");
    const tabStripRow = navigation.parentElement;
    const tabStripShell = tabStripRow?.parentElement;
    const shells = screen
      .getAllByRole("tab")
      .map((tab) => sessionTabShell(tab as HTMLElement));

    expect(tabStripShell?.className).not.toContain("flex-1");
    expect(tabStripRow?.className).not.toContain("flex-1");
    expect(navigation.className).toContain("overflow-x-auto");
    expect(navigation.className).toContain("overscroll-x-contain");
    expect(navigation.className).not.toContain("flex-1");
    expect(tablist.className).toContain("min-w-max");
    expect(tablist.className).toContain("inline-flex");
    for (const shell of shells) {
      expect(shell.className).toContain("flex-none");
      expect(shell.className).toContain("min-w-40");
      expect(shell.className).toContain("max-w-72");
    }
  });

  it("reorders session tabs with drag-and-drop and preserves the moved tab as active", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root/beta",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        target: {
          kind: "named",
          name: "beta",
        },
      },
      {
        factoryDir: "/workspace/root/gamma",
        folderPath: "/workspace/root/gamma",
        id: "session-gamma",
        isDefault: false,
        project: "gamma",
        target: {
          kind: "named",
          name: "gamma",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("navigation", { name: messages.sessionTabsLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("tab", { name: "beta" }));
    await waitFor(() => {
      expect(
        screen.getByRole("tab", { name: "beta" }).getAttribute("aria-selected"),
      ).toBe("true");
    });

    const rootShell = sessionTabShell(
      screen.getByRole("tab", { name: "root" }),
    );
    const gammaShell = sessionTabShell(
      screen.getByRole("tab", { name: "gamma" }),
    );
    const dataTransfer = {
      effectAllowed: "all",
      setData: vi.fn(),
    };

    Object.defineProperty(gammaShell, "getBoundingClientRect", {
      configurable: true,
      value: () => ({
        bottom: 40,
        height: 40,
        left: 200,
        right: 320,
        top: 0,
        width: 120,
        x: 200,
        y: 0,
        toJSON: () => "",
      }),
    });

    fireEvent.dragStart(rootShell, { dataTransfer });
    fireEvent.dragOver(gammaShell, { clientX: 319, dataTransfer });
    fireEvent.drop(gammaShell, { clientX: 319, dataTransfer });

    await waitFor(() => {
      expect(
        screen.getAllByRole("tab").map((tab) => tab.getAttribute("aria-label")),
      ).toEqual(["beta", "root", "gamma"]);
    });
    expect(
      screen.getByRole("tab", { name: "beta" }).getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("shows the offline state and allows session refetch", async () => {
    listFactorySessions
      .mockRejectedValueOnce(
        new FactorySessionsAPIError("network down", { code: "NETWORK_ERROR" }),
      )
      .mockResolvedValueOnce([]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(screen.getByText(messages.sessionsOfflineTitle)).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.retrySessionsLabel }),
    );

    await waitFor(() => {
      expect(screen.getByText(messages.sessionsEmptyTitle)).toBeTruthy();
    });
  });

  it("validates a folder inline and opens the only runnable target after confirmation", async () => {
    listFactorySessions
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
      ])
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
        {
          factoryDir: "/workspace/other",
          folderPath: "/workspace/other",
          id: "session-other",
          isDefault: false,
          project: "other",
          target: {
            kind: "default",
          },
        },
      ]);
    openFactorySession
      .mockResolvedValueOnce({
        targets: [
          {
            factoryDir: "/workspace/other",
            folderPath: "/workspace/other",
            label: "default",
            project: "other",
            ref: {
              kind: "default",
            },
          },
        ],
      })
      .mockResolvedValueOnce({
        session: {
          factoryDir: "/workspace/other",
          folderPath: "/workspace/other",
          id: "session-other",
          isDefault: false,
          project: "other",
          target: {
            kind: "default",
          },
        },
      });

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    expect(
      screen.getByText(messages.openSessionDialogDescription),
    ).toBeTruthy();
    const folderField = screen.getByRole("textbox", {
      name: messages.sessionFolderFieldLabel,
    });
    const inspectFolderButton = screen.getByRole("button", {
      name: messages.openSessionSubmitLabel,
    });
    expect(inspectFolderButton.getAttribute("disabled")).not.toBeNull();
    expect(folderField.getAttribute("aria-describedby")).toBeTruthy();
    expect(screen.getByText(messages.sessionFolderHelperText)).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: "/workspace/other" },
      },
    );
    expect(inspectFolderButton.getAttribute("disabled")).toBeNull();
    fireEvent.submit(inspectFolderButton.closest("form") as HTMLFormElement);

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: "/workspace/other",
        validateOnly: true,
      });
    });
    expect(
      screen.queryByText(messages.openSessionLaunchReadySingleTarget),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: messages.openSessionSubmitLabel }),
    ).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /default/i }));
    await waitFor(() => {
      expect(openFactorySession.mock.calls[1]?.[0]).toEqual({
        folderPath: "/workspace/other",
        target: {
          kind: "default",
        },
      });
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "other" })).toBeTruthy();
    });
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "session-other",
    );
  });

  it("reuses the validated resolved folder path when the customer entered a tilde path", async () => {
    listFactorySessions
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
      ])
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
        {
          factoryDir: "/Users/tester/factory-root/alpha",
          folderPath: "/Users/tester/factory-root",
          id: "session-alpha",
          isDefault: false,
          project: "alpha",
          target: {
            kind: "named",
            name: "alpha",
          },
        },
      ]);
    openFactorySession
      .mockResolvedValueOnce({
        targets: [
          {
            factoryDir: "/Users/tester/factory-root/alpha",
            folderPath: "/Users/tester/factory-root",
            label: "alpha",
            project: "alpha",
            ref: {
              kind: "named",
              name: "alpha",
            },
          },
        ],
      })
      .mockResolvedValueOnce({
        session: {
          factoryDir: "/Users/tester/factory-root/alpha",
          folderPath: "/Users/tester/factory-root",
          id: "session-alpha",
          isDefault: false,
          project: "alpha",
          target: {
            kind: "named",
            name: "alpha",
          },
        },
      });

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: "~/factory-root" },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: "~/factory-root",
        validateOnly: true,
      });
    });

    fireEvent.click(screen.getByRole("button", { name: /alpha/i }));

    await waitFor(() => {
      expect(openFactorySession.mock.calls[1]?.[0]).toEqual({
        folderPath: "/Users/tester/factory-root",
        target: {
          kind: "named",
          name: "alpha",
        },
      });
    });
  });

  it("shows clickable runnable target actions when the folder exposes multiple runnable targets", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
    ]);
    openFactorySession
      .mockResolvedValueOnce({
        targets: [
          {
            factoryDir: "/workspace/fleet",
            folderPath: "/workspace/fleet",
            label: "default",
            project: "fleet",
            ref: {
              kind: "default",
            },
          },
          {
            factoryDir: "/workspace/fleet/beta",
            folderPath: "/workspace/fleet",
            label: "beta",
            project: "beta",
            ref: {
              kind: "named",
              name: "beta",
            },
          },
        ],
      })
      .mockResolvedValueOnce({
        session: {
          factoryDir: "/workspace/fleet/beta",
          folderPath: "/workspace/fleet",
          id: "session-beta",
          isDefault: false,
          project: "beta",
          target: {
            kind: "named",
            name: "beta",
          },
        },
      });

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: "/workspace/fleet" },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: "/workspace/fleet",
        validateOnly: true,
      });
    });
    expect(
      screen.queryByText(messages.openSessionLaunchReadyMultipleTargets),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: messages.openSessionSubmitLabel }),
    ).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /beta/i }));

    await waitFor(() => {
      expect(openFactorySession.mock.calls[1]?.[0]).toEqual({
        folderPath: "/workspace/fleet",
        target: {
          kind: "named",
          name: "beta",
        },
      });
    });
  });

  it("shows recovery-oriented inline validation feedback for invalid folders", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
    ]);
    openFactorySession
      .mockRejectedValueOnce(
        new FactorySessionsAPIError("folder validation failed", {
          code: "BAD_REQUEST",
          targets: [
            factorySessionFieldTarget(
              "missing",
              "folderPath",
              "folder validation failed",
            ),
          ],
        }),
      )
      .mockRejectedValueOnce(
        new FactorySessionsAPIError("folder validation failed", {
          code: "BAD_REQUEST",
          targets: [
            factorySessionFieldTarget(
              "not_directory",
              "folderPath",
              "folder validation failed",
            ),
          ],
        }),
      )
      .mockRejectedValueOnce(
        new FactorySessionsAPIError("folder validation failed", {
          code: "BAD_REQUEST",
          targets: [
            factorySessionFieldTarget(
              "unreadable",
              "folderPath",
              "folder validation failed",
            ),
          ],
        }),
      )
      .mockRejectedValueOnce(
        new FactorySessionsAPIError("folder validation failed", {
          code: "BAD_REQUEST",
          targets: [
            factorySessionFieldTarget(
              "not_runnable",
              "folderPath",
              "folder validation failed",
            ),
          ],
        }),
      );

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );

    const folderField = screen.getByRole("textbox", {
      name: messages.sessionFolderFieldLabel,
    });
    const form = screen
      .getByRole("button", { name: messages.openSessionSubmitLabel })
      .closest("form") as HTMLFormElement;

    fireEvent.change(folderField, { target: { value: "/workspace/missing" } });
    fireEvent.submit(form);
    await waitFor(() => {
      expect(
        screen.getByText(
          "This folder does not exist yet. Check the path and choose an existing local factory folder.",
        ),
      ).toBeTruthy();
    });

    fireEvent.change(folderField, {
      target: { value: "/workspace/factory.yaml" },
    });
    fireEvent.submit(form);
    await waitFor(() => {
      expect(
        screen.getByText(
          "This path points to a file instead of a folder. Choose a local factory folder that contains a runnable factory.",
        ),
      ).toBeTruthy();
    });

    fireEvent.change(folderField, { target: { value: "/workspace/private" } });
    fireEvent.submit(form);
    await waitFor(() => {
      expect(
        screen.getByText(
          "This folder could not be read from this machine. Check its permissions, then choose a readable local factory folder.",
        ),
      ).toBeTruthy();
    });

    fireEvent.change(folderField, { target: { value: "/workspace/empty" } });
    fireEvent.submit(form);
    await waitFor(() => {
      expect(
        screen.getByText(
          "This folder was found, but it does not contain a runnable factory. Choose a folder with a runnable factory and try again.",
        ),
      ).toBeTruthy();
    });
    expect(openFactorySession).toHaveBeenCalledTimes(4);
    expect(
      screen.queryByRole("region", { name: messages.targetPickerTitle }),
    ).toBeNull();
  });

  it("confirms init-new-factory after validateOnly returns initsNewFactory", async () => {
    const selectedFolderPath = "/workspace/new-factory-root";
    const nestedFactoryPath = `${selectedFolderPath}/factory`;
    listFactorySessions
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
      ])
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
        {
          factoryDir: nestedFactoryPath,
          folderPath: selectedFolderPath,
          id: "session-new-factory",
          isDefault: false,
          project: "new-factory-root",
          target: {
            kind: "named",
            name: "factory",
          },
        },
      ]);
    openFactorySession
      .mockResolvedValueOnce({
        folderPath: selectedFolderPath,
        initsNewFactory: true,
      })
      .mockResolvedValueOnce({
        session: {
          factoryDir: nestedFactoryPath,
          folderPath: selectedFolderPath,
          id: "session-new-factory",
          isDefault: false,
          project: "new-factory-root",
          target: {
            kind: "named",
            name: "factory",
          },
        },
      });

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: selectedFolderPath },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: selectedFolderPath,
        validateOnly: true,
      });
    });
    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: messages.openSessionCreateFactoryLabel,
        }),
      ).toBeTruthy();
    });
    expect(
      screen.getByText(
        messages.openSessionInitNewFactoryDescriptionTemplate.replaceAll(
          "{{folderPath}}",
          selectedFolderPath,
        ),
      ),
    ).toBeTruthy();
    expect(screen.getByText(nestedFactoryPath)).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: messages.openSessionSubmitLabel }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: messages.openSessionCreateFactoryLabel,
      }),
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[1]?.[0]).toEqual({
        folderPath: selectedFolderPath,
        initNewFactory: true,
      });
    });
    await waitFor(() => {
      expect(
        screen.getByRole("tab", { name: "new-factory-root" }),
      ).toBeTruthy();
    });
    const createdTab = screen.getByRole("tab", { name: "new-factory-root" });
    expect(createdTab.textContent).toContain(
      sessionTabSecondaryPath(selectedFolderPath),
    );
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "session-new-factory",
    );
    expect(openFactorySession).toHaveBeenCalledTimes(2);
  });

  it("reopens a nested init-new-factory session through the selected folder path", async () => {
    const selectedFolderPath = "/workspace/reopen-project";
    const nestedFactoryPath = `${selectedFolderPath}/factory`;
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: nestedFactoryPath,
        folderPath: selectedFolderPath,
        id: "session-reopen-project",
        isDefault: false,
        project: "reopen-project",
        target: {
          kind: "named",
          name: "factory",
        },
      },
    ]);
    openFactorySession.mockResolvedValueOnce({
      targets: [
        {
          factoryDir: nestedFactoryPath,
          folderPath: selectedFolderPath,
          label: "factory",
          project: "reopen-project",
          ref: {
            kind: "named",
            name: "factory",
          },
        },
      ],
    });

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "reopen-project" })).toBeTruthy();
    });

    const reopenTab = screen.getByRole("tab", { name: "reopen-project" });
    expect(reopenTab.textContent).toContain(
      sessionTabSecondaryPath(selectedFolderPath),
    );
    expect(reopenTab.textContent).not.toContain("factory/factory");

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: selectedFolderPath },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: selectedFolderPath,
        validateOnly: true,
      });
    });
    await waitFor(() => {
      expect(screen.getByText(nestedFactoryPath)).toBeTruthy();
    });
  });

  it("returns to folder entry when canceling init-new-factory confirmation", async () => {
    const emptyFolderPath = "/workspace/cancel-init-root";
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
    ]);
    openFactorySession.mockResolvedValueOnce({
      folderPath: emptyFolderPath,
      initsNewFactory: true,
    });

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: emptyFolderPath },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: messages.openSessionCreateFactoryLabel,
        }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: messages.openSessionCancelCreateFactoryLabel,
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionSubmitLabel }),
      ).toBeTruthy();
    });
    expect(
      screen.queryByRole("button", {
        name: messages.openSessionCreateFactoryLabel,
      }),
    ).toBeNull();
    expect(openFactorySession).toHaveBeenCalledTimes(1);
    expect(
      (
        screen.getByRole("textbox", {
          name: messages.sessionFolderFieldLabel,
        }) as HTMLInputElement
      ).value,
    ).toBe(emptyFolderPath);
  });

  it("shows inline config-load diagnostics instead of init-new-factory actions for broken folders", async () => {
    const selectedFolderPath = "/workspace/broken-project";
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
    ]);
    openFactorySession.mockRejectedValueOnce(
      new FactorySessionsAPIError(
        "factory configuration could not be loaded from the selected folder",
        {
          code: "FACTORY_SESSION_CONFIG_LOAD_FAILED",
          targets: [
            factorySessionTargetTarget(
              "config_load_failed",
              "default",
              'Factory target "default" at "/workspace/broken-project" could not be loaded: unexpected end of JSON input',
            ),
          ],
        },
      ),
    );

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: selectedFolderPath },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "factory configuration could not be loaded from the selected folder",
        ),
      ).toBeTruthy();
    });

    expect(
      screen.getByText(
        'Factory target "default" at "/workspace/broken-project" could not be loaded: unexpected end of JSON input',
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: messages.openSessionCreateFactoryLabel,
      }),
    ).toBeNull();
    expect(
      screen.queryByText(messages.openSessionFolderUnknownError),
    ).toBeNull();
    expect(screen.queryByRole("tab", { name: "broken-project" })).toBeNull();
  });

  it("keeps config-load diagnostics inline when the selected target fails during open", async () => {
    const selectedFolderPath = "/workspace/broken-project";
    const targetLoadMessage =
      'Factory target "review" at "/workspace/broken-project/review" could not be loaded: unknown workstation runner';
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
    ]);
    openFactorySession
      .mockResolvedValueOnce({
        targets: [
          {
            factoryDir: "/workspace/broken-project/review",
            folderPath: selectedFolderPath,
            label: "review",
            project: "broken-project",
            ref: {
              kind: "named",
              name: "review",
            },
          },
        ],
      })
      .mockRejectedValueOnce(
        new FactorySessionsAPIError(
          "factory configuration could not be loaded from the selected folder",
          {
            code: "FACTORY_SESSION_CONFIG_LOAD_FAILED",
            targets: [
              factorySessionTargetTarget(
                "config_load_failed",
                "review",
                targetLoadMessage,
              ),
            ],
          },
        ),
      );

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);
    const messages = getHeaderControlsMessages("en");

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.openSessionButtonLabel }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    );
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: selectedFolderPath },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(screen.getByText("/workspace/broken-project/review")).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: /review.*\/workspace\/broken-project\/review/i,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText(targetLoadMessage)).toBeTruthy();
    });

    expect(
      screen.queryByRole("button", {
        name: messages.openSessionCreateFactoryLabel,
      }),
    ).toBeNull();
    expect(screen.queryByRole("tab", { name: "broken-project" })).toBeNull();
  });

  it("closes the active session tab and selects the remaining session deterministically", async () => {
    listFactorySessions
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
        {
          factoryDir: "/workspace/root/beta",
          folderPath: "/workspace/root",
          id: "session-beta",
          isDefault: false,
          project: "beta",
          target: {
            kind: "named",
            name: "beta",
          },
        },
      ])
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root/beta",
          folderPath: "/workspace/root",
          id: "session-beta",
          isDefault: false,
          project: "beta",
          target: {
            kind: "named",
            name: "beta",
          },
        },
      ]);
    closeFactorySession.mockResolvedValue(undefined);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "root" })).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: "Close root session",
      }),
    );

    await waitFor(() => {
      expect(closeFactorySession).toHaveBeenCalledWith("~default");
    });
    await waitFor(() => {
      expect(
        screen.getByRole("tab", { name: "beta" }).getAttribute("aria-selected"),
      ).toBe("true");
    });
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "session-beta",
    );
  });

  it("keeps the active session close control attached to the active tab", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        target: {
          kind: "named",
          name: "beta",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);

    const activeTab = await screen.findByRole("tab", { name: "root" });
    const activeCluster = screen.getByRole("button", {
      name: "Close root session",
    }).parentElement;

    expect(
      activeCluster?.contains(
        screen.getByRole("button", { name: "Close root session" }),
      ),
    ).toBe(true);
    expect(
      activeTab.parentElement?.contains(activeCluster as HTMLElement),
    ).toBe(true);
    expect(
      screen.getByRole("button", { name: "Close beta session" }),
    ).toBeTruthy();
  });

  it("keeps inactive-tab close buttons quiet by default and directly operable", async () => {
    listFactorySessions
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
        {
          factoryDir: "/workspace/root/beta",
          folderPath: "/workspace/root",
          id: "session-beta",
          isDefault: false,
          project: "beta",
          target: {
            kind: "named",
            name: "beta",
          },
        },
        {
          factoryDir: "/workspace/root/gamma",
          folderPath: "/workspace/root",
          id: "session-gamma",
          isDefault: false,
          project: "gamma",
          target: {
            kind: "named",
            name: "gamma",
          },
        },
      ])
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
        {
          factoryDir: "/workspace/root/gamma",
          folderPath: "/workspace/root",
          id: "session-gamma",
          isDefault: false,
          project: "gamma",
          target: {
            kind: "named",
            name: "gamma",
          },
        },
      ]);
    closeFactorySession.mockResolvedValue(undefined);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);

    const betaCloseButton = await screen.findByRole("button", {
      name: "Close beta session",
    });

    expect(betaCloseButton.className).toContain("text-on-surface-disabled");
    expect(betaCloseButton.className).toContain(
      "group-hover:text-on-surface-variant",
    );
    expect(betaCloseButton.className).toContain(
      "focus-visible:ring-af-focus-ring",
    );

    betaCloseButton.focus();
    expect(document.activeElement).toBe(betaCloseButton);
    expect(
      screen.getByRole("tab", { name: "root" }).getAttribute("aria-selected"),
    ).toBe("true");
    expect(
      screen.getByRole("tab", { name: "beta" }).getAttribute("aria-selected"),
    ).toBe("false");

    fireEvent.click(betaCloseButton);

    await waitFor(() => {
      expect(closeFactorySession).toHaveBeenCalledWith("session-beta");
    });
    expect(
      screen.getByRole("tab", { name: "root" }).getAttribute("aria-selected"),
    ).toBe("true");
    expect(screen.queryByRole("tab", { name: "beta" })).toBeNull();
  });

  it("keeps the open-session affordance available when the last session is closed", async () => {
    listFactorySessions
      .mockResolvedValueOnce([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: "~default",
          isDefault: true,
          project: "root",
          target: {
            kind: "default",
          },
        },
      ])
      .mockResolvedValueOnce([]);
    closeFactorySession.mockResolvedValue(undefined);
    const messages = getHeaderControlsMessages("en");

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "root" })).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: "Close root session",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText(messages.sessionsEmptyTitle)).toBeTruthy();
    });
    expect(useDashboardSessionStore.getState().selectedSessionID).toBeNull();
    expect(
      screen.getByRole("button", { name: messages.openSessionButtonLabel }),
    ).toBeTruthy();
  });

  it("keeps close actions icon-only while a session close request is pending", async () => {
    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        target: {
          kind: "default",
        },
      },
    ]);
    closeFactorySession.mockImplementation(
      () => new Promise<void>(() => undefined),
    );

    renderWithQueryClient(<DashboardSessionTabs locale="ja" />);
    const messages = getHeaderControlsMessages("ja");
    const rootSession = {
      factoryDir: "/workspace/root",
      folderPath: "/workspace/root",
      id: "~default",
      isDefault: true,
      project: "root",
      target: {
        kind: "default" as const,
      },
    };

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "root" })).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: sessionCloseLabel(rootSession, messages),
      }),
    );

    const pendingCloseButton = await screen.findByRole("button", {
      name: sessionCloseLabel(rootSession, messages),
    });

    expect((pendingCloseButton as HTMLButtonElement).disabled).toBe(true);
    expect(pendingCloseButton.textContent?.trim()).toBe(
      messages.closingSessionButtonLabel,
    );
  });

  it("shows the default session tab secondary line from absolute folderPath metadata", async () => {
    const absoluteFactoryPath =
      "/Users/operator/infinite-you/agent-factory/examples/catalog/factory";

    listFactorySessions.mockResolvedValue([
      {
        factoryDir: absoluteFactoryPath,
        folderPath: absoluteFactoryPath,
        id: "~default",
        isDefault: true,
        project: "factory",
        target: {
          kind: "default",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "factory" })).toBeTruthy();
    });

    const defaultTab = screen.getByRole("tab", { name: "factory" });
    const truncatedSecondaryPath = sessionTabSecondaryPath(
      absoluteFactoryPath,
      SESSION_TAB_PATH_MAX_LENGTH,
    );

    expect(defaultTab.getAttribute("aria-label")).toBe("factory");
    expect(screen.getByText(truncatedSecondaryPath)).toBeTruthy();
    expect(screen.getByTitle(absoluteFactoryPath).textContent).toBe(
      truncatedSecondaryPath,
    );
    expect(defaultTab.parentElement?.getAttribute("title")).toBe(
      `${absoluteFactoryPath} (~default)`,
    );
  });

  it("uses short factory-first tab labels while preserving visible supporting context", async () => {
    const longFolderPath =
      "/workspace/customers/northwind/agent-factory/examples/catalog";

    listFactorySessions.mockResolvedValue([
      {
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "workspace root",
        target: {
          kind: "default",
        },
      },
      {
        factoryDir: "/workspace/catalog/review",
        folderPath: longFolderPath,
        id: "session-review",
        isDefault: false,
        project: "catalog",
        target: {
          kind: "named",
          name: "review",
        },
      },
    ]);

    renderWithQueryClient(<DashboardSessionTabs locale="en" />);

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "root" })).toBeTruthy();
    });

    const rootTab = screen.getByRole("tab", { name: "root" });
    const reviewTab = screen.getByRole("tab", { name: "review" });

    expect(rootTab.getAttribute("aria-label")).toBe("root");
    expect(reviewTab.getAttribute("aria-label")).toBe("review");
    expect(rootTab.textContent).toBe("root/workspace/root");
    expect(reviewTab.textContent).toBe(
      "review...mers/northwind/agent-factory/examples/catalog",
    );
    expect(screen.getByText("/workspace/root")).toBeTruthy();
    expect(screen.getByTitle(longFolderPath).textContent).toBe(
      "...mers/northwind/agent-factory/examples/catalog",
    );
  });
});

function sessionTabShell(tab: HTMLElement): HTMLElement {
  const shell = tab.parentElement;
  if (!shell) {
    throw new Error("Expected session tab shell wrapper");
  }
  return shell;
}

function expectSubtleActiveSessionTabShell(shell: HTMLElement) {
  expect(shell.className).toContain("bg-surface-container-low");
  expect(shell.className).toContain("text-on-surface");
  expect(shell.className.includes("border-outline-variant")).toBe(false);
  expect(shell.className.includes("bg-surface-container-high")).toBe(false);
  expect(shell.className.includes("bg-primary")).toBe(false);
  expect(shell.className.includes("bg-primary-container")).toBe(false);
}

function expectMutedInactiveSessionTabShell(shell: HTMLElement) {
  expect(shell.className).toContain("text-on-surface-variant");
  expect(shell.className.includes("bg-surface-container-low")).toBe(false);
  expect(shell.className.includes("bg-surface-container-high")).toBe(false);
  expect(shell.className.includes("bg-primary")).toBe(false);
  expect(shell.className.includes("bg-primary-container")).toBe(false);
  expect(shell.className.includes("hover:border-outline")).toBe(false);
}

function renderWithQueryClient(view: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}
