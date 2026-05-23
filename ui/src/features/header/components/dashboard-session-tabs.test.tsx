// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: existing dashboard-session-tabs coverage stayed intact during feature-root migration.
// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: existing dashboard-session-tabs coverage stayed intact during feature-root migration.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";

import { FactorySessionsAPIError } from "../../../api/factory-sessions";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { DashboardSessionTabs } from "./dashboard-session-tabs";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { sessionCloseLabel } from "../lib/dashboard-session-tabs-utils";

const listFactorySessions = vi.fn();
const openFactorySession = vi.fn();
const closeFactorySession = vi.fn();

vi.mock("../../../api/factory-sessions", () => ({
  FactorySessionsAPIError: class FactorySessionsAPIError extends Error {
    public readonly code: string;
    public readonly responseBody?: unknown;
    public readonly status?: number;
    public readonly statusText?: string;

    public constructor(
      message: string,
      details: {
        code: string;
        responseBody?: unknown;
        status?: number;
        statusText?: string;
      },
    ) {
      super(message);
      this.name = "FactorySessionsAPIError";
      this.code = details.code;
      this.responseBody = details.responseBody;
      this.status = details.status;
      this.statusText = details.statusText;
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
    expect(screen.getByRole("tabpanel")).toBeTruthy();
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
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe("~default");
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

  it("opens a folder and auto-activates the created session when one target exists", async () => {
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
    openFactorySession.mockResolvedValue({
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
    expect(screen.getByText(messages.openSessionDialogDescription)).toBeTruthy();
    const folderField = screen.getByRole("textbox", {
      name: messages.sessionFolderFieldLabel,
    });
    expect(folderField.getAttribute("aria-describedby")).toBeTruthy();
    expect(screen.getByText(messages.sessionFolderHelperText)).toBeTruthy();
    fireEvent.change(
      screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder),
      {
        target: { value: "/workspace/other" },
      },
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: "/workspace/other",
      });
    });
    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "other" })).toBeTruthy();
    });
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "session-other",
    );
  });

  it("populates the folder field from the browser directory picker before opening a session", async () => {
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
          factoryDir: "/workspace/fleet",
          folderPath: "/workspace/fleet",
          id: "session-fleet",
          isDefault: false,
          project: "fleet",
          target: {
            kind: "default",
          },
        },
      ]);
    openFactorySession.mockResolvedValue({
      session: {
        factoryDir: "/workspace/fleet",
        folderPath: "/workspace/fleet",
        id: "session-fleet",
        isDefault: false,
        project: "fleet",
        target: {
          kind: "default",
        },
      },
    });
    const showDirectoryPicker = vi.fn().mockResolvedValue({
      kind: "directory",
      name: "fleet",
      path: "/workspace/fleet",
    } satisfies Partial<FileSystemDirectoryHandle>);
    vi.stubGlobal("showDirectoryPicker", showDirectoryPicker);

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

    fireEvent.click(
      screen.getByRole("button", {
        name: messages.browseSessionFolderButtonLabel,
      }),
    );

    await waitFor(() => {
      expect(showDirectoryPicker).toHaveBeenCalledTimes(1);
    });
    expect(
      (
        screen.getByPlaceholderText(
          messages.sessionFolderFieldPlaceholder,
        ) as HTMLInputElement
      ).value,
    ).toBe("/workspace/fleet");

    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: "/workspace/fleet",
      });
    });
  });

  it("falls back to the hidden directory input when the browser picker is unavailable", async () => {
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
          factoryDir: "/workspace/fleet",
          folderPath: "/workspace/fleet",
          id: "session-fleet",
          isDefault: false,
          project: "fleet",
          target: {
            kind: "default",
          },
        },
      ]);
    openFactorySession.mockResolvedValue({
      session: {
        factoryDir: "/workspace/fleet",
        folderPath: "/workspace/fleet",
        id: "session-fleet",
        isDefault: false,
        project: "fleet",
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
    fireEvent.click(
      screen.getByRole("button", {
        name: messages.browseSessionFolderButtonLabel,
      }),
    );

    const folderPickerInput = document.body.querySelector(
      'input[type="file"][webkitdirectory]',
    ) as HTMLInputElement | null;
    expect(folderPickerInput).toBeTruthy();

    const pickedFile = new File(["factory"], "factory.yaml", {
      type: "text/yaml",
    });
    Object.defineProperty(pickedFile, "path", {
      value: "/workspace/fleet/factory.yaml",
    });

    fireEvent.change(folderPickerInput as HTMLInputElement, {
      target: {
        files: [pickedFile],
      },
    });

    expect(
      (
        screen.getByPlaceholderText(
          messages.sessionFolderFieldPlaceholder,
        ) as HTMLInputElement
      ).value,
    ).toBe("/workspace/fleet");

    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: "/workspace/fleet",
      });
    });
  });

  it("ignores relative directory picker names and waits for an absolute folder path", async () => {
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
          factoryDir: "/workspace/fleet",
          folderPath: "/workspace/fleet",
          id: "session-fleet",
          isDefault: false,
          project: "fleet",
          target: {
            kind: "default",
          },
        },
      ]);
    openFactorySession.mockResolvedValue({
      session: {
        factoryDir: "/workspace/fleet",
        folderPath: "/workspace/fleet",
        id: "session-fleet",
        isDefault: false,
        project: "fleet",
        target: {
          kind: "default",
        },
      },
    });
    const showDirectoryPicker = vi.fn().mockResolvedValue({
      kind: "directory",
      name: "infinite-you",
    } satisfies Partial<FileSystemDirectoryHandle>);
    vi.stubGlobal("showDirectoryPicker", showDirectoryPicker);

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
    fireEvent.click(
      screen.getByRole("button", {
        name: messages.browseSessionFolderButtonLabel,
      }),
    );

    await waitFor(() => {
      expect(showDirectoryPicker).toHaveBeenCalledTimes(1);
    });
    expect(
      (
        screen.getByPlaceholderText(
          messages.sessionFolderFieldPlaceholder,
        ) as HTMLInputElement
      ).value,
    ).toBe("");

    const folderPickerInput = document.body.querySelector(
      'input[type="file"][webkitdirectory]',
    ) as HTMLInputElement | null;
    expect(folderPickerInput).toBeTruthy();

    const pickedFile = new File(["factory"], "factory.yaml", {
      type: "text/yaml",
    });
    Object.defineProperty(pickedFile, "path", {
      value: "/workspace/fleet/factory.yaml",
    });

    fireEvent.change(folderPickerInput as HTMLInputElement, {
      target: {
        files: [pickedFile],
      },
    });

    expect(
      (
        screen.getByPlaceholderText(
          messages.sessionFolderFieldPlaceholder,
        ) as HTMLInputElement
      ).value,
    ).toBe("/workspace/fleet");
  });

  it("shows a compact target picker when the folder exposes multiple runnable targets", async () => {
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
    fireEvent.change(screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder), {
      target: { value: "/workspace/fleet" },
    });
    fireEvent.submit(
      screen
        .getByRole("button", { name: messages.openSessionSubmitLabel })
        .closest("form") as HTMLFormElement,
    );

    const targetPicker = await screen.findByRole("region", {
      name: messages.targetPickerTitle,
    });
    const picker = within(targetPicker);

    fireEvent.click(picker.getByRole("button", { name: /beta/i }));

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
        screen.getByRole("tab", { name: "beta" }).getAttribute(
          "aria-selected",
        ),
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
    expect(activeTab.parentElement?.contains(activeCluster as HTMLElement)).toBe(true);
    expect(screen.getByRole("button", { name: "Close beta session" })).toBeTruthy();
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

    expect(betaCloseButton.className).toContain("text-af-ink/34");
    expect(betaCloseButton.className).toContain("group-focus-within:text-af-ink/76");
    expect(betaCloseButton.className).toContain("focus-visible:text-af-ink");

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
      name: messages.closingSessionButtonLabel,
    });

    expect(
      (pendingCloseButton as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(pendingCloseButton.textContent?.trim()).toBe("");
    expect(screen.queryByText(messages.closingSessionButtonLabel)).toBeNull();
  });

  it("uses short factory-first labels without rendering redundant visible subtitle copy", async () => {
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
        folderPath: "/workspace/catalog",
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

    expect(rootTab.textContent).toBe("root");
    expect(reviewTab.textContent).toBe("review");
    expect(screen.queryByText("workspace root")).toBeNull();
    expect(screen.queryByText("catalog")).toBeNull();
  });
});

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
