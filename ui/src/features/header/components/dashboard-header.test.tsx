import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import { AppLocaleProvider, NATIVE_LANGUAGE_LABELS } from "../../../i18n";
import {
  AppColorPaletteProvider,
  applyDocumentColorPalette,
} from "../../../theme";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import { getExportDialogMessages } from "../../export/messages/export-dialog";
import { useExportDialogStore } from "../../export/state/exportDialogStore";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { getColorPaletteOptions } from "../messages/color-palette-options";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardHeader } from "./dashboard-header";

vi.mock("./dashboard-session-tabs", () => ({
  DashboardSessionTabs: ({ locale }: { locale: string }) => (
    <div data-testid={`dashboard-session-tabs-${locale}`}>
      <div>Dashboard session tabs {locale}</div>
      <button
        aria-label={getHeaderControlsMessages(locale).openSessionButtonLabel}
        type="button"
      >
        +
      </button>
    </div>
  ),
}));

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
    window.sessionStorage.clear();
    applyDocumentColorPalette("factory-dark");
    useExportDialogStore.setState({ isExportDialogOpen: false });
    useFactoryTimelineStore.getState().reset();
    useDashboardStreamStore.setState({
      streamState: {
        message: "Connecting to the event stream.",
        status: "connecting",
      },
    });
  });

  it("renders shared neutral header action buttons and opens the export dialog state", () => {
    seedDashboardHeaderSnapshot();

    renderWithQueryClient(<DashboardHeader />);
    const messages = getExportDialogMessages("en");
    const headerMessages = getHeaderControlsMessages("en");
    const toolbar = screen.getByRole("region", {
      name: headerMessages.dashboardSummaryLabel,
    });
    const heading = screen.getByRole("heading");
    const slider = screen.getByRole("slider", {
      name: headerMessages.sliderAriaLabel,
    });
    const languageButton = screen.getByRole<HTMLButtonElement>("button", {
      name: headerMessages.languageMenuButtonLabel,
    });
    const paletteButton = screen.getByRole<HTMLButtonElement>("button", {
      name: headerMessages.paletteMenuButtonLabel,
    });
    const openSessionButton = screen.getByRole<HTMLButtonElement>("button", {
      name: headerMessages.openSessionButtonLabel,
    });
    const globalActions = screen.getByRole("group", {
      name: headerMessages.globalHeaderActionsLabel,
    });
    const actionRow = globalActions.parentElement?.parentElement;
    const actionRowSections = toolbar.querySelectorAll(
      "[data-action-row-section]",
    );

    const exportButton = screen.getByRole<HTMLButtonElement>("button", {
      name: messages.triggerLabel,
    });
    expect(exportButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(exportButton.getAttribute("aria-haspopup")).toBe("dialog");
    expect(exportButton.getAttribute("aria-expanded")).toBe("false");
    expect(toolbar.dataset.dashboardPanelShell).toBe("panel");
    expect(toolbar.className).toContain("border-outline");
    expect(toolbar.className).toContain("bg-surface-container-high");
    expect(toolbar.className).toContain("mb-3");
    expect(toolbar.className).toContain("gap-2");
    expect(toolbar.className).toContain("p-2");
    expect(toolbar.firstElementChild?.className).toContain("grid");
    expect(toolbar.firstElementChild?.className).toContain("md:grid-cols");
    const topRegion = toolbar.querySelector(
      "[data-dashboard-header-top-region]",
    );
    const tabRegion = toolbar.querySelector(
      "[data-dashboard-header-tab-region]",
    );
    const controlRegion = toolbar.querySelector(
      "[data-dashboard-header-control-region]",
    );
    expect(topRegion?.className).toContain("justify-between");
    expect(topRegion?.className).toContain("md:contents");
    expect(tabRegion?.className).toContain("md:col-start-2");
    expect(controlRegion?.className).toContain("min-w-0");
    expect(heading.textContent).toContain("U");
    expect(
      toolbar.firstElementChild?.firstElementChild?.firstElementChild,
    ).toBe(heading);
    expect(heading.className).toContain("md:col-start-1");
    expect(heading.firstElementChild?.className).toContain("items-center");
    expect(globalActions.className).toContain("self-end");
    expect(actionRow?.className).toContain("justify-end");
    expect(actionRow?.className).toContain("md:col-start-3");
    expect(actionRowSections).toHaveLength(1);
    expect(actionRowSections[0]?.getAttribute("data-action-row-section")).toBe(
      "actions",
    );
    expect(actionRowSections[0]?.contains(languageButton)).toBe(true);
    expect(actionRowSections[0]?.contains(paletteButton)).toBe(true);
    expect(
      paletteButton.compareDocumentPosition(languageButton) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(heading.firstElementChild?.firstElementChild?.className).toContain(
      "h-12",
    );
    expect(slider.closest("div")?.parentElement?.className).toContain(
      "rounded-t-2xl",
    );
    expect(slider.closest("div")?.parentElement?.className).toContain(
      "bg-surface-container-low",
    );
    expect(slider.closest("div")?.parentElement?.className).toContain("w-full");
    expect(slider.closest("div")?.className).toContain("md:flex-nowrap");
    expect(slider.closest("div")?.className).toContain("w-full");
    const controls = Array.from(
      toolbar.querySelectorAll(
        `[aria-label="${headerMessages.sliderAriaLabel}"], [aria-label="${headerMessages.openSessionButtonLabel}"], [aria-label="${headerMessages.languageMenuButtonLabel}"], [aria-label="${messages.triggerLabel}"]`,
      ),
    );
    expect(controls).toHaveLength(4);
    expect(controls[0]).toBe(languageButton);
    expect(controls[1]).toBe(openSessionButton);
    expect(controls[2]).toBe(slider);
    expect(controls[3]).toBe(exportButton);
    expect(globalActions.contains(languageButton)).toBe(true);
    expect(globalActions.contains(paletteButton)).toBe(true);
    expect(globalActions.contains(openSessionButton)).toBe(false);
    expect(globalActions.contains(exportButton)).toBe(false);
    expect(actionRowSections[0]?.contains(exportButton)).toBe(false);
    expect(
      globalActions.compareDocumentPosition(slider) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(languageButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(paletteButton.dataset.dashboardHeaderAction).toBe("neutral");
    expect(languageButton.getAttribute("aria-haspopup")).toBe("menu");
    expect(paletteButton.getAttribute("aria-haspopup")).toBe("menu");
    expect(languageButton.getAttribute("aria-expanded")).toBe("false");
    expect(languageButton.className).toContain("h-10");
    expect(languageButton.className).toContain("w-10");
    expect(openSessionButton.textContent).toBe("+");
    expect(exportButton.className).toContain("h-10");
    expect(exportButton.className).toContain("w-10");
    expect(screen.getByText("Dashboard session tabs en")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: headerMessages.returnToCurrentTickLabel,
      }),
    ).toBeNull();
    expect(useExportDialogStore.getState().isExportDialogOpen).toBe(false);

    act(() => {
      fireEvent.click(exportButton);
    });

    return waitFor(() => {
      expect(useExportDialogStore.getState().isExportDialogOpen).toBe(true);
      expect(exportButton.getAttribute("aria-expanded")).toBe("true");
    });
  });

  it("resolves the export trigger accessible name from the export locale catalog", () => {
    seedDashboardHeaderSnapshot();

    const messages = getExportDialogMessages("ja");
    renderWithQueryClient(<DashboardHeader locale="ja" />);

    expect(
      screen.getByRole("button", { name: messages.triggerLabel }),
    ).toBeTruthy();
  });

  it("keeps global header controls icon-only while preserving localized accessible names", () => {
    seedDashboardHeaderSnapshot();

    const headerMessages = getHeaderControlsMessages("ko");
    const exportMessages = getExportDialogMessages("ko");

    renderWithQueryClient(<DashboardHeader locale="ko" />);

    expect(
      screen.getByRole("button", {
        name: headerMessages.openSessionButtonLabel,
      }).textContent,
    ).toBe("+");
    expect(
      screen.getByRole("button", {
        name: exportMessages.triggerLabel,
      }).textContent,
    ).toBe("");
    expect(
      screen.getByRole("button", {
        name: headerMessages.languageMenuButtonLabel,
      }).textContent,
    ).toBe("");
    expect(
      screen.queryByText(headerMessages.openSessionButtonLabel),
    ).toBeNull();
    expect(screen.queryByText(exportMessages.triggerLabel)).toBeNull();
    expect(
      screen.getByRole("button", {
        name: headerMessages.paletteMenuButtonLabel,
      }).textContent,
    ).toBe("");
    expect(
      screen.queryByText(headerMessages.paletteMenuButtonLabel),
    ).toBeNull();
    expect(
      screen.queryByText(headerMessages.languageMenuButtonLabel),
    ).toBeNull();
    expect(screen.queryByText(headerMessages.sliderLabel)?.className).toContain(
      "sr-only",
    );
  });

  it("resolves the header summary, brand, slider, and session-status labels from the requested locale catalog", () => {
    seedDashboardHeaderSnapshot();
    act(() => {
      useDashboardStreamStore.setState({
        streamState: {
          message: "Event stream is offline.",
          status: "offline",
        },
      });
    });

    const messages = getHeaderControlsMessages("zh-CN");

    renderWithQueryClient(<DashboardHeader locale="zh-CN" />);

    expect(
      screen.getByRole("region", { name: messages.dashboardSummaryLabel }),
    ).toBeTruthy();
    expect(screen.getByRole("heading")).toBeTruthy();
    expect(screen.getByText(messages.sliderLabel).className).toContain(
      "sr-only",
    );
    expect(
      screen.getByRole("slider", { name: messages.sliderAriaLabel }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: messages.returnToCurrentTickLabel }),
    ).toBeNull();
    expect(screen.getByText("Dashboard session tabs zh-CN")).toBeTruthy();
    expect(
      screen.queryByRole("status", { name: messages.streamStatusOfflineLabel }),
    ).toBeNull();
  });

  it("opens and closes the locale menu through keyboard and dismissal events", async () => {
    seedDashboardHeaderSnapshot();

    renderWithQueryClient(<DashboardHeader />);

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
        name: NATIVE_LANGUAGE_LABELS.en,
      }).className,
    ).toContain("rounded-xl");
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

    renderWithQueryClient(
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

    expect(screen.getByText("1/2")).toBeTruthy();
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
    expect(screen.getByText("1/2")).toBeTruthy();
    expect(
      screen.getByRole("region", {
        name: koreanMessages.dashboardSummaryLabel,
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
      screen
        .getByRole("menuitemradio", {
          name: NATIVE_LANGUAGE_LABELS.ko,
        })
        .getAttribute("aria-checked"),
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
    expect(screen.getByText("1/2")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: englishMessages.returnToCurrentTickLabel,
      }),
    ).toBeNull();
    expect(
      screen.getByRole("button", {
        name: englishExportMessages.triggerLabel,
      }),
    ).toBeTruthy();
  });

  it("switches the dashboard palette without reloading and persists it for the session", async () => {
    seedDashboardHeaderSnapshot();
    window.sessionStorage.clear();

    renderWithQueryClient(
      <AppColorPaletteProvider>
        <DashboardHeader />
      </AppColorPaletteProvider>,
    );

    const messages = getHeaderControlsMessages("en");
    const paletteButton = screen.getByRole("button", {
      name: messages.paletteMenuButtonLabel,
    });

    fireEvent.click(paletteButton);

    expect(
      screen.getByRole("menu", { name: messages.paletteLabel }),
    ).toBeTruthy();
    expect(
      screen.getAllByRole("menuitemradio").map((item) => item.textContent),
    ).toEqual(getColorPaletteOptions("en").map((option) => option.label));

    fireEvent.click(screen.getByRole("menuitemradio", { name: "Slate" }));

    await waitFor(() => {
      expect(document.documentElement.dataset.colorPalette).toBe("slate");
    });
    expect(
      window.sessionStorage.getItem("infinite-you-dashboard-color-palette"),
    ).toBe("slate");
    expect(
      screen.queryByRole("menu", { name: messages.paletteLabel }),
    ).toBeNull();
  });

  it("opens, navigates, and dismisses the palette menu with keyboard and pointer checkpoints", async () => {
    seedDashboardHeaderSnapshot();
    window.sessionStorage.clear();

    renderWithQueryClient(
      <AppColorPaletteProvider>
        <DashboardHeader />
      </AppColorPaletteProvider>,
    );

    const messages = getHeaderControlsMessages("en");
    const paletteButton = screen.getByRole("button", {
      name: messages.paletteMenuButtonLabel,
    });

    fireEvent.keyDown(paletteButton, { key: "ArrowDown" });

    const paletteMenu = screen.getByRole("menu", {
      name: messages.paletteLabel,
    });
    const menuItems = screen.getAllByRole("menuitemradio");
    expect(document.activeElement).toBe(menuItems[0]);

    fireEvent.keyDown(paletteMenu, { key: "ArrowDown" });
    expect(document.activeElement).toBe(menuItems[1]);

    fireEvent.keyDown(paletteMenu, { key: "Escape" });
    await waitFor(() => {
      expect(
        screen.queryByRole("menu", { name: messages.paletteLabel }),
      ).toBeNull();
    });
    expect(document.activeElement).toBe(paletteButton);

    fireEvent.click(paletteButton);
    expect(
      screen.getByRole("menu", { name: messages.paletteLabel }),
    ).toBeTruthy();

    fireEvent.pointerDown(document.body);
    await waitFor(() => {
      expect(
        screen.queryByRole("menu", { name: messages.paletteLabel }),
      ).toBeNull();
    });
    expect(document.activeElement).toBe(paletteButton);
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
