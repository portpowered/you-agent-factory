import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { FactorySessionsAPIError } from "../../api/factory-sessions";
import { DashboardSessionTabs } from "./dashboard-session-tabs";
import { getHeaderControlsMessages } from "./messages/header-controls";

const listFactorySessions = vi.fn();
const openFactorySession = vi.fn();

vi.mock("../../api/factory-sessions", () => ({
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
  openFactorySession: (...args: unknown[]) => openFactorySession(...args),
}));

describe("DashboardSessionTabs", () => {
  beforeEach(() => {
    listFactorySessions.mockReset();
    openFactorySession.mockReset();
  });

  it("renders loading and then the active session tabs with folder inspection text", async () => {
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

    expect(screen.getByRole("button", { name: /root \/ default/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /root \/ beta/i })).toBeTruthy();
    expect(
      screen.getByText(`${messages.activeSessionPathLabel}: /workspace/root`),
    ).toBeTruthy();
  });

  it("supports keyboard navigation across session tabs", async () => {
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

    const rootTab = screen.getByRole("button", { name: /root \/ default/i });
    const betaTab = screen.getByRole("button", { name: /root \/ beta/i });
    const gammaTab = screen.getByRole("button", { name: /root \/ gamma/i });

    expect(rootTab.getAttribute("aria-pressed")).toBe("true");
    rootTab.focus();

    fireEvent.keyDown(rootTab, { key: "ArrowRight" });
    await waitFor(() => {
      expect(betaTab.getAttribute("aria-pressed")).toBe("true");
    });
    expect(document.activeElement).toBe(betaTab);

    fireEvent.keyDown(betaTab, { key: "End" });
    await waitFor(() => {
      expect(gammaTab.getAttribute("aria-pressed")).toBe("true");
    });
    expect(document.activeElement).toBe(gammaTab);

    fireEvent.keyDown(gammaTab, { key: "Home" });
    await waitFor(() => {
      expect(rootTab.getAttribute("aria-pressed")).toBe("true");
    });
    expect(document.activeElement).toBe(rootTab);
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
    fireEvent.change(screen.getByPlaceholderText(messages.sessionFolderFieldPlaceholder), {
      target: { value: "/workspace/other" },
    });
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
      expect(screen.getByRole("button", { name: /other \/ default/i })).toBeTruthy();
    });
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
