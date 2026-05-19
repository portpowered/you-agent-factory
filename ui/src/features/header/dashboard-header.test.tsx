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
import { AppLocaleProvider, NATIVE_LANGUAGE_LABELS } from "../../i18n";
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
    const languageButton = screen.getByRole<HTMLButtonElement>("button", {
      name: headerMessages.languageMenuButtonLabel,
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
    expect(languageButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(languageButton.getAttribute("aria-haspopup")).toBe("menu");
    expect(languageButton.getAttribute("aria-expanded")).toBe("false");
    expect(languageButton.className).toContain("h-10");
    expect(languageButton.className).toContain("w-10");
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

  it("opens and closes the locale menu through keyboard and dismissal events", async () => {
    seedDashboardHeaderSnapshot();

    render(<DashboardHeader />);

    const messages = getHeaderControlsMessages("en");
    const languageButton = screen.getByRole("button", {
      name: messages.languageMenuButtonLabel,
    });

    languageButton.focus();
    act(() => {
      fireEvent.keyDown(languageButton, { key: "Enter" });
    });

    expect(
      screen.getByRole("menu", { name: messages.languageLabel }),
    ).toBeTruthy();
    expect(languageButton.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.en,
      }),
    );
    expect(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS["zh-CN"],
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.ko,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.ja,
      }),
    ).toBeTruthy();

    act(() => {
      fireEvent.keyDown(
        screen.getByRole("menu", { name: messages.languageLabel }),
        {
          key: "Escape",
        },
      );
    });

    await waitFor(() => {
      expect(
        screen.queryByRole("menu", { name: messages.languageLabel }),
      ).toBeNull();
    });
    expect(document.activeElement).toBe(languageButton);
    expect(languageButton.getAttribute("aria-expanded")).toBe("false");
  });

  it("switches the header locale across the supported languages through session state", async () => {
    seedDashboardHeaderSnapshot();

    render(
      <AppLocaleProvider initialLocale="en">
        <DashboardHeader />
      </AppLocaleProvider>,
    );

    const englishMessages = getHeaderControlsMessages("en");
    const englishExportMessages = getExportDialogMessages("en");
    const koreanMessages = getHeaderControlsMessages("ko");
    const koreanExportMessages = getExportDialogMessages("ko");
    const languageButton = screen.getByRole("button", {
      name: englishMessages.languageMenuButtonLabel,
    });

    expect(screen.getByText("Tick 1 of 2")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: englishExportMessages.triggerLabel,
      }),
    ).toBeTruthy();
    expect(languageButton.className).toContain("h-10");
    expect(languageButton.className).toContain("rounded-lg");

    fireEvent.click(languageButton);
    fireEvent.click(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.ko,
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: koreanMessages.languageMenuButtonLabel,
        }),
      ).toBeTruthy();
    });
    expect(screen.getByText("틱 1 / 2")).toBeTruthy();
    expect(
      screen.getByRole("region", {
        name: koreanMessages.dashboardSummaryLabel,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("status", {
        name: koreanMessages.streamStatusConnectingLabel,
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: koreanExportMessages.triggerLabel,
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("menu", { name: koreanMessages.languageLabel }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: koreanMessages.languageMenuButtonLabel,
      }),
    );
    expect(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.ko,
      }).getAttribute("aria-checked"),
    ).toBe("true");
    expect(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS["zh-CN"],
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.ja,
      }),
    ).toBeTruthy();

    fireEvent.click(
      screen.getByRole("menuitemradio", {
        name: NATIVE_LANGUAGE_LABELS.en,
      }),
    );

    expect(
      screen.getByRole("button", {
        name: englishMessages.languageMenuButtonLabel,
      }),
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
