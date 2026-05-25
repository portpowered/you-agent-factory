import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { graphStateSmokeTimelineEvents } from "../../../components/dashboard/fixtures";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import {
  getHeaderControlsMessages,
  HEADER_CURRENT_TICK_TOKEN,
  HEADER_MAX_TICK_TOKEN,
} from "../messages/header-controls";
import { TickSliderControl } from "./tick-slider-control";

type TimelineWorldState = ReturnType<
  typeof useFactoryTimelineStore.getState
>["worldViewCache"][number];

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

function currentTickStatus(
  locale: string,
  currentTick: number,
  maxTick: number,
): string {
  const messages = getHeaderControlsMessages(locale);

  return messages.currentTickStatusTemplate
    .replaceAll(HEADER_CURRENT_TICK_TOKEN, String(currentTick))
    .replaceAll(HEADER_MAX_TICK_TOKEN, String(maxTick));
}

describe("TickSliderControl", () => {
  afterEach(() => {
    act(() => {
      useFactoryTimelineStore.getState().reset();
    });
  });

  it("renders an explained disabled state until more than one tick is available", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    act(() => {
      useFactoryTimelineStore.getState().replaceEvents([
        timelineEvent("tick-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
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
        }),
      ]);
    });

    const messages = getHeaderControlsMessages("en");

    try {
      render(<TickSliderControl />);

      expect(
        screen.getByRole<HTMLInputElement>("slider", {
          name: messages.sliderAriaLabel,
        }).disabled,
      ).toBe(true);
      expect(screen.getByText(messages.waitingForMoreTicks)).toBeTruthy();
      expect(
        screen.queryByRole("button", {
          name: messages.returnToCurrentTickLabel,
        }),
      ).toBeNull();
      expect(screen.queryByText("Current")).toBeNull();
      expect(
        errorSpy.mock.calls.some(([firstArg]) =>
          String(firstArg).includes("not wrapped in act"),
        ),
      ).toBe(false);
    } finally {
      errorSpy.mockRestore();
    }
  });

  it("switches between fixed and current mode through scrubber interaction", async () => {
    useFactoryTimelineStore
      .getState()
      .replaceEvents(graphStateSmokeTimelineEvents);

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });
    const sliderShell = slider.closest("div");
    const metaGroup = screen.getByText("9/9").parentElement;
    const statusText = screen.getByText("9/9");

    expect(slider.value).toBe("9");
    expect(statusText).toBeTruthy();
    expect(slider.getAttribute("aria-describedby")).toBe(statusText.id);
    expect(slider.getAttribute("aria-valuetext")).toBe("9/9");
    expect(sliderShell?.className).toContain("gap-1.5");
    expect(sliderShell?.className).toContain("px-1");
    expect(sliderShell?.className).toContain("py-1");
    expect(sliderShell?.className).not.toContain("border-af-border");
    expect(sliderShell?.className).not.toContain("bg-af-surface-subtle");
    expect(sliderShell?.className).not.toContain("rounded-lg");
    expect(sliderShell?.className).not.toContain("md:w-auto");
    expect(sliderShell?.className).not.toContain("md:max-w-lg");
    expect(screen.queryByText("Current")).toBeNull();
    expect(useFactoryTimelineStore.getState().mode).toBe("current");
    expect(
      screen.queryByRole("button", {
        name: messages.returnToCurrentTickLabel,
      }),
    ).toBeNull();

    fireEvent.change(slider, { target: { value: "2" } });

    await waitFor(() => {
      expect(screen.getByText("2/9")).toBeTruthy();
    });
    expect(slider.getAttribute("aria-valuetext")).toBe("2/9");
    expect(useFactoryTimelineStore.getState().mode).toBe("fixed");
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(2);

    fireEvent.change(slider, { target: { value: "9" } });

    await waitFor(() => {
      expect(screen.getByText("9/9")).toBeTruthy();
    });
    expect(useFactoryTimelineStore.getState().mode).toBe("current");
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(9);
    expect(metaGroup?.className).toContain("items-center");
  });

  it("falls back to zero bounds when no timeline ticks are available", () => {
    act(() => {
      useFactoryTimelineStore.setState({
        events: [],
        latestTick: 0,
        mode: "fixed",
        selectedTick: 7,
        worldViewCache: {} as Record<number, TimelineWorldState>,
      });
    });

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });

    expect(slider.disabled).toBe(true);
    expect(slider.min).toBe("0");
    expect(slider.max).toBe("0");
    expect(slider.value).toBe("0");
    expect(slider.getAttribute("aria-valuetext")).toBe(
      messages.waitingForMoreTicks,
    );
    expect(screen.getByText(messages.waitingForMoreTicks)).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: messages.returnToCurrentTickLabel,
      }),
    ).toBeNull();
  });

  it("ignores non-numeric cached ticks and clamps the selected tick to cached bounds", () => {
    act(() => {
      useFactoryTimelineStore.setState({
        events: [],
        latestTick: 0,
        mode: "fixed",
        selectedTick: 9,
        worldViewCache: {
          2: {} as TimelineWorldState,
          4: {} as TimelineWorldState,
          NaN: {} as TimelineWorldState,
        } as Record<number, TimelineWorldState>,
      });
    });

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });

    expect(slider.disabled).toBe(false);
    expect(slider.min).toBe("2");
    expect(slider.max).toBe("4");
    expect(slider.value).toBe("4");
    expect(screen.getByText("4/4")).toBeTruthy();
    expect(useFactoryTimelineStore.getState().mode).toBe("fixed");
  });

  it("renders the slider copy from the requested locale catalog", async () => {
    useFactoryTimelineStore
      .getState()
      .replaceEvents(graphStateSmokeTimelineEvents);

    const messages = getHeaderControlsMessages("ja");

    render(<TickSliderControl locale="ja" />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });

    expect(screen.getByText(messages.sliderLabel).className).toContain(
      "sr-only",
    );
    expect(screen.getByText(currentTickStatus("ja", 9, 9))).toBeTruthy();

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(screen.getByText(currentTickStatus("ja", 3, 9))).toBeTruthy();
    });
  });

  it("keeps the visible timeline label hidden while preserving the slider accessible name", () => {
    useFactoryTimelineStore
      .getState()
      .replaceEvents(graphStateSmokeTimelineEvents);

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    expect(
      screen.getByRole<HTMLInputElement>("slider", {
        name: messages.sliderAriaLabel,
      }),
    ).toBeTruthy();
    expect(screen.getByText(messages.sliderLabel).className).toContain(
      "sr-only",
    );
  });

  it("keeps the slider status semantically attached without extra visible copy", () => {
    act(() => {
      useFactoryTimelineStore
        .getState()
        .replaceEvents(graphStateSmokeTimelineEvents);
    });

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });
    const statusText = screen.getByText("9/9");

    expect(slider.getAttribute("aria-describedby")).toBe(statusText.id);
    expect(statusText.tagName).toBe("OUTPUT");
    expect(screen.queryByText("Current")).toBeNull();
  });

  it("keeps the tick status in one compact cluster beside the scrubber", () => {
    act(() => {
      useFactoryTimelineStore
        .getState()
        .replaceEvents(graphStateSmokeTimelineEvents);
    });

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });
    const statusText = screen.getByText("9/9");
    const metaGroup = statusText.parentElement;

    expect(metaGroup).toBeTruthy();
    expect(metaGroup?.contains(statusText)).toBe(true);
    expect(statusText.className).toContain("tabular-nums");
    expect(slider.compareDocumentPosition(metaGroup as Node)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  });

  it("renders the localized disabled-state copy from the requested locale catalog", () => {
    act(() => {
      useFactoryTimelineStore.getState().replaceEvents([
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
      ]);
    });

    const messages = getHeaderControlsMessages("ja");

    render(<TickSliderControl locale="ja" />);

    expect(
      screen.getByRole<HTMLInputElement>("slider", {
        name: messages.sliderAriaLabel,
      }).disabled,
    ).toBe(true);
    expect(screen.getByText(messages.waitingForMoreTicks)).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: messages.returnToCurrentTickLabel,
      }),
    ).toBeNull();
  });
});
