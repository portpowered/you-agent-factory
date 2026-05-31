import "../../../../testing/bun-factory-sessions-api-mocks";
import { beforeEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  listFactorySessionsMock,
  openFactorySessionMock,
} from "../../../../testing/bun-factory-sessions-api-mocks";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { DashboardSessionTabs } from "./dashboard-session-tabs";
import { getHeaderControlsMessages } from "../messages/header-controls";

describe("DashboardSessionTabs launch target selection", () => {
  beforeEach(() => {
    listFactorySessionsMock.mockReset();
    openFactorySessionMock.mockReset();
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  it("launches the selected named factory instead of falling back to the default target", async () => {
    listFactorySessionsMock
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
    openFactorySessionMock
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
      expect(openFactorySessionMock.mock.calls[0]?.[0]).toEqual({
        folderPath: "/workspace/fleet",
        validateOnly: true,
      });
    });

    fireEvent.click(screen.getByRole("button", { name: /beta/i }));

    await waitFor(() => {
      expect(openFactorySessionMock.mock.calls[1]?.[0]).toEqual({
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
