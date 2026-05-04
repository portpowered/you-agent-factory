import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../api/events";
import { DashboardHeader } from "./dashboard-header";
import { useDashboardStreamStore } from "../dashboard/state/dashboardStreamStore";
import { useExportDialogStore } from "../export/state/exportDialogStore";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";

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

    const exportButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Export PNG",
    });
    const currentButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Return to current tick",
    });

    expect(exportButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(currentButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(exportButton.getAttribute("aria-haspopup")).toBe("dialog");
    expect(exportButton.getAttribute("aria-expanded")).toBe("false");
    expect(useExportDialogStore.getState().isExportDialogOpen).toBe(false);

    fireEvent.click(exportButton);

    return waitFor(() => {
      expect(useExportDialogStore.getState().isExportDialogOpen).toBe(true);
      expect(exportButton.getAttribute("aria-expanded")).toBe("true");
    });
  });
});
