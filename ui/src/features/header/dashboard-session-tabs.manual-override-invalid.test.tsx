import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";

import { FactorySessionsAPIError } from "../../api/factory-sessions";
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

describe("DashboardSessionTabs invalid manual override", () => {
  beforeEach(() => {
    listFactorySessions.mockReset();
    openFactorySession.mockReset();
    closeFactorySession.mockReset();
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  it("shows an inline error and blocks launch when the manual override target is invalid", async () => {
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
      new FactorySessionsAPIError("target validation failed", {
        code: "BAD_REQUEST",
        targets: [
          {
            field: "target.name",
            id: "target_not_found",
            kind: "factory-session-validation",
          },
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
        target: { value: "gamma" },
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
        target: {
          kind: "named",
          name: "gamma",
        },
        validateOnly: true,
      });
    });
    expect(
      screen.getByText(
        "This factory name is not launchable from the chosen folder. Check the name or clear the override and use a detected target.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: messages.openSessionTargetLabel }),
    ).toBeNull();
  });
});

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
