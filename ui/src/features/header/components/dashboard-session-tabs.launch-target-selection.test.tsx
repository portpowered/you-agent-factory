import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardSessionTabs } from "./dashboard-session-tabs";

const STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS =
  "border-outline bg-surface-container-high text-on-surface hover:border-outline-variant hover:bg-af-overlay";
const STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS =
  "border-outline-variant bg-surface-container-low text-on-surface";

const listFactorySessions = vi.fn();
const openFactorySession = vi.fn();
const closeFactorySession = vi.fn();

vi.mock("../../../api/factory-sessions", () => ({
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

function mockLaunchTargetSelectionSessionApis() {
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
}

describe("DashboardSessionTabs launch target selection", () => {
  beforeEach(() => {
    listFactorySessions.mockReset();
    openFactorySession.mockReset();
    closeFactorySession.mockReset();
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  it("launches the selected named factory instead of falling back to the default target", async () => {
    mockLaunchTargetSelectionSessionApis();
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

    const defaultTargetButton = screen.getByRole("button", {
      name: /default/i,
    });
    const betaTargetButton = screen.getByRole("button", { name: /beta/i });

    expectStandardListTargetRow(defaultTargetButton, { selected: false });
    expectStandardListTargetRow(betaTargetButton, { selected: false });

    fireEvent.click(betaTargetButton);
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

    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "session-beta",
    );
  });
});

const ACCENT_SELECTED_TOKENS = [
  "bg-primary",
  "bg-primary-container",
  "border-primary",
  "shadow-af-accent-selected",
] as const;

function expectStandardListTargetRow(
  row: HTMLElement,
  { selected }: { selected: boolean },
) {
  expect(row.getAttribute("aria-pressed")).toBe(selected ? "true" : "false");
  expect(row.getAttribute("data-selected")).toBe(selected ? "true" : "false");
  expect(row.className).toContain(
    selected
      ? STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS
      : STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS,
  );
  for (const token of ACCENT_SELECTED_TOKENS) {
    expect(row.className).not.toContain(token);
  }
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
