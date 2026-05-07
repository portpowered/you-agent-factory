import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../api/events";
import { useDashboardStreamStore } from "../dashboard/state/dashboardStreamStore";
import { getExportDialogMessages } from "../export/messages/export-dialog";
import { useExportDialogStore } from "../export/state/exportDialogStore";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";
import { DashboardHeader } from "./dashboard-header";
import { getHeaderControlsMessages } from "./messages/header-controls";

function timelineEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-16T12:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

describe("DashboardHeader", () => {
  afterEach(() => {
    useExportDialogStore.setState({ isExportDialogOpen: false });
    useFactoryTimelineStore.getState().reset();
    useDashboardStreamStore.setState({
      streamState: {
        message: "Connecting to the Infinite You event stream.",
        status: "connecting",
      },
    });
  });

  it("renders shared neutral header action buttons and opens the export dialog state", () => {
    act(() => {
      useFactoryTimelineStore.setState({
        events: [
          timelineEvent(
            "tick-1",
            1,
            FACTORY_EVENT_TYPES.initialStructureRequest,
            {
              factory: {
                workTypes: [
                  {
                    name: "story",
                    states: [{ name: "ready", type: "INITIAL" }],
                  },
                ],
                workstations: [],
                workers: [],
              },
            },
          ),
          timelineEvent(
            "tick-2",
            2,
            FACTORY_EVENT_TYPES.initialStructureRequest,
            {
              factory: {
                workTypes: [
                  {
                    name: "story",
                    states: [{ name: "ready", type: "INITIAL" }],
                  },
                ],
                workstations: [],
                workers: [],
              },
            },
          ),
        ],
        latestTick: 2,
        mode: "fixed",
        selectedTick: 1,
        worldViewCache: {
          1: {} as never,
          2: {} as never,
        },
      });
    });

    render(<DashboardHeader />);
    const messages = getExportDialogMessages("en");
    const headerMessages = getHeaderControlsMessages("en");
    const toolbar = screen.getByRole("region", {
      name: headerMessages.dashboardSummaryLabel,
    });
    const heading = screen.getByRole("heading", { name: "Infinite You" });
    const wordmark = screen.getByText("Infinite You");
    const slider = screen.getByRole("slider", {
      name: headerMessages.sliderAriaLabel,
    });

    const exportButton = screen.getByRole<HTMLButtonElement>("button", {
      name: messages.triggerLabel,
    });
    const currentButton = screen.getByRole<HTMLButtonElement>("button", {
      name: headerMessages.returnToCurrentTickLabel,
    });
    const streamStatus = screen.getByRole("status", {
      name: headerMessages.streamStatusConnectingLabel,
    });

    expect(exportButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(currentButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(exportButton.getAttribute("aria-haspopup")).toBe("dialog");
    expect(exportButton.getAttribute("aria-expanded")).toBe("false");
    expect(wordmark.className).toContain("sr-only");
    expect(heading.textContent).toContain("∞");
    expect(heading.textContent).toContain("U");
    expect(toolbar.firstElementChild).toBe(heading);
    expect(slider.closest("div")?.parentElement?.className).toContain(
      "justify-end",
    );
    expect(streamStatus).toBeTruthy();
    expect(useExportDialogStore.getState().isExportDialogOpen).toBe(false);

    fireEvent.click(exportButton);

    return waitFor(() => {
      expect(useExportDialogStore.getState().isExportDialogOpen).toBe(true);
      expect(exportButton.getAttribute("aria-expanded")).toBe("true");
    });
  });

  it("resolves the export trigger accessible name from the export locale catalog", () => {
    act(() => {
      useFactoryTimelineStore.setState({
        events: [
          timelineEvent(
            "tick-1",
            1,
            FACTORY_EVENT_TYPES.initialStructureRequest,
            {
              factory: {
                workTypes: [
                  {
                    name: "story",
                    states: [{ name: "ready", type: "INITIAL" }],
                  },
                ],
                workstations: [],
                workers: [],
              },
            },
          ),
        ],
        latestTick: 1,
        mode: "fixed",
        selectedTick: 1,
        worldViewCache: {
          1: {} as never,
        },
      });
    });

    const messages = getExportDialogMessages("ja");
    render(<DashboardHeader locale="ja" />);

    expect(
      screen.getByRole("button", { name: messages.triggerLabel }),
    ).toBeTruthy();
  });

  it("resolves the header summary, slider, and stream-status labels from the requested locale catalog", () => {
    act(() => {
      useFactoryTimelineStore.setState({
        events: [
          timelineEvent(
            "tick-1",
            1,
            FACTORY_EVENT_TYPES.initialStructureRequest,
            {
              factory: {
                workTypes: [
                  {
                    name: "story",
                    states: [{ name: "ready", type: "INITIAL" }],
                  },
                ],
                workstations: [],
                workers: [],
              },
            },
          ),
          timelineEvent(
            "tick-2",
            2,
            FACTORY_EVENT_TYPES.initialStructureRequest,
            {
              factory: {
                workTypes: [
                  {
                    name: "story",
                    states: [{ name: "ready", type: "INITIAL" }],
                  },
                ],
                workstations: [],
                workers: [],
              },
            },
          ),
        ],
        latestTick: 2,
        mode: "fixed",
        selectedTick: 1,
        worldViewCache: {
          1: {} as never,
          2: {} as never,
        },
      });
      useDashboardStreamStore.setState({
        streamState: {
          message: "Infinite You event stream is offline.",
          status: "offline",
        },
      });
    });

    const messages = getHeaderControlsMessages("ja");

    render(<DashboardHeader locale="ja" />);

    expect(
      screen.getByRole("region", { name: messages.dashboardSummaryLabel }),
    ).toBeTruthy();
    expect(
      screen.getByRole("slider", { name: messages.sliderAriaLabel }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: messages.returnToCurrentTickLabel }),
    ).toBeTruthy();
    expect(
      screen.getByRole("status", { name: messages.streamStatusOfflineLabel }),
    ).toBeTruthy();
  });

  it("renders each localized stream-status accessible name from the header catalog", () => {
    act(() => {
      useFactoryTimelineStore.setState({
        events: [
          timelineEvent(
            "tick-1",
            1,
            FACTORY_EVENT_TYPES.initialStructureRequest,
            {
              factory: {
                workTypes: [
                  {
                    name: "story",
                    states: [{ name: "ready", type: "INITIAL" }],
                  },
                ],
                workstations: [],
                workers: [],
              },
            },
          ),
          timelineEvent(
            "tick-2",
            2,
            FACTORY_EVENT_TYPES.initialStructureRequest,
            {
              factory: {
                workTypes: [
                  {
                    name: "story",
                    states: [{ name: "ready", type: "INITIAL" }],
                  },
                ],
                workstations: [],
                workers: [],
              },
            },
          ),
        ],
        latestTick: 2,
        mode: "current",
        selectedTick: 2,
        worldViewCache: {
          1: {} as never,
          2: {} as never,
        },
      });
    });

    const messages = getHeaderControlsMessages("ja");
    const statuses = [
      {
        label: messages.streamStatusConnectingLabel,
        status: "connecting" as const,
      },
      { label: messages.streamStatusLiveLabel, status: "live" as const },
      {
        label: messages.streamStatusOfflineLabel,
        status: "offline" as const,
      },
    ];

    for (const { label, status } of statuses) {
      act(() => {
        useDashboardStreamStore.setState({
          streamState: {
            message: `stream is ${status}`,
            status,
          },
        });
      });

      cleanup();
      render(<DashboardHeader locale="ja" />);

      expect(screen.getByRole("status", { name: label })).toBeTruthy();
    }
  });
});
