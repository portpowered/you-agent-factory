import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../api/session-routing";
import { useDashboardSessionStore } from "../dashboard/state/dashboardSessionStore";
import { DashboardSessionTabs } from "./components/dashboard-session-tabs";
import { getHeaderControlsMessages } from "./messages/header-controls";

const listFactorySessions = vi.fn();
const openFactorySession = vi.fn();
const closeFactorySession = vi.fn();

vi.mock("../../api/factory-sessions", () => ({
  FactorySessionsAPIError: class FactorySessionsAPIError extends Error {
    public readonly code: string;
    public readonly targets?: unknown[];

    public constructor(
      message: string,
      details: { code: string; targets?: unknown[] },
    ) {
      super(message);
      this.name = "FactorySessionsAPIError";
      this.code = details.code;
      this.targets = details.targets;
    }
  },
  listFactorySessions: (...args: unknown[]) => listFactorySessions(...args),
  closeFactorySession: (...args: unknown[]) => closeFactorySession(...args),
  openFactorySession: (...args: unknown[]) => openFactorySession(...args),
}));

describe("DashboardSessionTabs manual override", () => {
  beforeEach(() => {
    listFactorySessions.mockReset();
    openFactorySession.mockReset();
    closeFactorySession.mockReset();
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  it("uses the manual named-factory override instead of the detected selection", async () => {
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

    openDialog(messages);
    fireEvent.change(
      screen.getByRole("textbox", {
        name: messages.sessionFolderFieldLabel,
      }),
      {
        target: { value: "/workspace/fleet" },
      },
    );
    fireEvent.change(
      screen.getByRole("textbox", {
        name: messages.manualFactoryNameFieldLabel,
      }),
      {
        target: { value: "beta" },
      },
    );
    submitDialog(messages);

    await waitFor(() => {
      expect(openFactorySession.mock.calls[0]?.[0]).toEqual({
        folderPath: "/workspace/fleet",
        target: {
          kind: "named",
          name: "beta",
        },
        validateOnly: true,
      });
    });
    expect(
      screen.getByText(
        "Manual override beta will launch instead of the detected selection.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Launch will use folder /workspace/fleet and factory beta.",
      ),
    ).toBeTruthy();

    fireEvent.change(
      screen.getByRole("combobox", {
        name: messages.selectSessionTargetLabel,
      }),
      { target: { value: "default" } },
    );
    fireEvent.click(
      screen.getByRole("button", { name: messages.openSessionTargetLabel }),
    );

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

function openDialog(messages: ReturnType<typeof getHeaderControlsMessages>) {
  fireEvent.click(
    screen.getByRole("button", { name: messages.openSessionButtonLabel }),
  );
}

function submitDialog(messages: ReturnType<typeof getHeaderControlsMessages>) {
  fireEvent.submit(
    screen
      .getByRole("button", { name: messages.openSessionSubmitLabel })
      .closest("form") as HTMLFormElement,
  );
}

function renderWithQueryClient(view: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}
