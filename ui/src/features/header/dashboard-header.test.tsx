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
import { AppLocaleProvider } from "../../i18n";
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

function seedDashboardHeaderSnapshot() {
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
    seedDashboardHeaderSnapshot();

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
    expect(exportButton.className).toContain("h-10");
    expect(exportButton.className).toContain("w-10");
    expect(currentButton.className).toContain("h-10");
    expect(currentButton.className).toContain("w-10");
    expect(streamStatus.className).toContain("h-10");
    expect(streamStatus.className).toContain("w-10");
    expect(streamStatus).toBeTruthy();
    expect(useExportDialogStore.getState().isExportDialogOpen).toBe(false);

    fireEvent.click(exportButton);

    return waitFor(() => {
      expect(useExportDialogStore.getState().isExportDialogOpen).toBe(true);
      expect(exportButton.getAttribute("aria-expanded")).toBe("true");
    });
  });

  it("resolves the export trigger accessible name from the export locale catalog", () => {
    seedDashboardHeaderSnapshot();

    const messages = getExportDialogMessages("ja");
    render(<DashboardHeader locale="ja" />);

    expect(
      screen.getByRole("button", { name: messages.triggerLabel }),
    ).toBeTruthy();
  });

  it("resolves the header summary, brand, slider, and stream-status labels from the requested locale catalog", () => {
    seedDashboardHeaderSnapshot();
    act(() => {
      useDashboardStreamStore.setState({
        streamState: {
          message: "Infinite You event stream is offline.",
          status: "offline",
        },
      });
    });

    const messages = getHeaderControlsMessages("zh-CN");

    render(<DashboardHeader locale="zh-CN" />);

    expect(
      screen.getByRole("region", { name: messages.dashboardSummaryLabel }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: messages.brandWordmark }),
    ).toBeTruthy();
    expect(screen.getByText(messages.sliderLabel)).toBeTruthy();
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
    seedDashboardHeaderSnapshot();
    act(() => {
      useFactoryTimelineStore.setState({
        mode: "current",
        selectedTick: 2,
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

  it("switches the header locale between English and Mandarin through session state", () => {
    seedDashboardHeaderSnapshot();

    render(
      <AppLocaleProvider initialLocale="en">
        <DashboardHeader />
      </AppLocaleProvider>,
    );

    const englishMessages = getHeaderControlsMessages("en");
    const englishExportMessages = getExportDialogMessages("en");
    const mandarinMessages = getHeaderControlsMessages("zh-CN");
    const mandarinExportMessages = getExportDialogMessages("zh-CN");
    const switcher = screen.getByRole("combobox", {
      name: englishMessages.languageLabel,
    });

    expect(screen.getByText("Tick 1 of 2")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: englishExportMessages.triggerLabel,
      }),
    ).toBeTruthy();
    expect(switcher.className).toContain("min-h-10");
    expect(switcher.className).toContain("rounded-lg");

    fireEvent.change(switcher, { target: { value: "zh-CN" } });

    expect(
      screen.getByRole("combobox", { name: mandarinMessages.languageLabel }),
    ).toBeTruthy();
    expect(screen.getByText("第 1 个刻度，共 2 个")).toBeTruthy();
    expect(
      screen.getByRole("region", {
        name: mandarinMessages.dashboardSummaryLabel,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("status", {
        name: mandarinMessages.streamStatusConnectingLabel,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: mandarinExportMessages.triggerLabel,
      }),
    ).toBeTruthy();

    fireEvent.change(
      screen.getByRole("combobox", { name: mandarinMessages.languageLabel }),
      { target: { value: "en" } },
    );

    expect(
      screen.getByRole("combobox", { name: englishMessages.languageLabel }),
    ).toBeTruthy();
    expect(screen.getByText("Tick 1 of 2")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: englishMessages.returnToCurrentTickLabel,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: englishExportMessages.triggerLabel,
      }),
    ).toBeTruthy();
  });
});
