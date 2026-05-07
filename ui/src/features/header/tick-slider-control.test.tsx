import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { FactoryEvent } from "../../api/events";
import { FACTORY_EVENT_TYPES } from "../../api/events";
import { graphStateSmokeTimelineEvents } from "../../components/dashboard/fixtures";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";
import {
  getHeaderControlsMessages,
  HEADER_CURRENT_TICK_TOKEN,
  HEADER_MAX_TICK_TOKEN,
} from "./messages/header-controls";
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
    useFactoryTimelineStore.getState().reset();
  });

  it("renders an explained disabled state until more than one tick is available", () => {
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

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    expect(
      screen.getByRole<HTMLInputElement>("slider", {
        name: messages.sliderAriaLabel,
      }).disabled,
    ).toBe(true);
    expect(screen.getByText(messages.waitingForMoreTicks)).toBeTruthy();
    const currentButton = screen.getByRole<HTMLButtonElement>("button", {
      name: messages.returnToCurrentTickLabel,
    });

    expect(currentButton.disabled).toBe(true);
    expect(screen.queryByText("Current")).toBeNull();
  });

  it("switches between fixed and current mode through the rendered controls", async () => {
    useFactoryTimelineStore
      .getState()
      .replaceEvents(graphStateSmokeTimelineEvents);

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });
    const currentButton = screen.getByRole<HTMLButtonElement>("button", {
      name: messages.returnToCurrentTickLabel,
    });

    expect(slider.value).toBe("9");
    expect(screen.getByText("Tick 9 of 9")).toBeTruthy();
    expect(currentButton.disabled).toBe(true);
    expect(currentButton.className).toContain("opacity-75");
    expect(screen.queryByText("Current")).toBeNull();
    expect(useFactoryTimelineStore.getState().mode).toBe("current");

    fireEvent.change(slider, { target: { value: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Tick 2 of 9")).toBeTruthy();
    });
    expect(currentButton.disabled).toBe(false);
    expect(currentButton.className).not.toContain("opacity-75");
    expect(useFactoryTimelineStore.getState().mode).toBe("fixed");
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(2);

    fireEvent.click(currentButton);

    await waitFor(() => {
      expect(screen.getByText("Tick 9 of 9")).toBeTruthy();
    });
    expect(currentButton.disabled).toBe(true);
    expect(useFactoryTimelineStore.getState().mode).toBe("current");
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(9);
  });

  it("falls back to zero bounds when no timeline ticks are available", () => {
    useFactoryTimelineStore.setState({
      events: [],
      latestTick: 0,
      mode: "fixed",
      selectedTick: 7,
      worldViewCache: {} as Record<number, TimelineWorldState>,
    });

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });
    const currentButton = screen.getByRole<HTMLButtonElement>("button", {
      name: messages.returnToCurrentTickLabel,
    });

    expect(slider.disabled).toBe(true);
    expect(slider.min).toBe("0");
    expect(slider.max).toBe("0");
    expect(slider.value).toBe("0");
    expect(screen.getByText(messages.waitingForMoreTicks)).toBeTruthy();
    expect(currentButton.disabled).toBe(true);
  });

  it("ignores non-numeric cached ticks and clamps the selected tick to cached bounds", () => {
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

    const messages = getHeaderControlsMessages("en");

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: messages.sliderAriaLabel,
    });
    const currentButton = screen.getByRole<HTMLButtonElement>("button", {
      name: messages.returnToCurrentTickLabel,
    });

    expect(slider.disabled).toBe(false);
    expect(slider.min).toBe("2");
    expect(slider.max).toBe("4");
    expect(slider.value).toBe("4");
    expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
    expect(currentButton.disabled).toBe(false);
    expect(currentButton.className).not.toContain("opacity-75");
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
    const currentButton = screen.getByRole<HTMLButtonElement>("button", {
      name: messages.returnToCurrentTickLabel,
    });

    expect(screen.getByText(messages.sliderLabel)).toBeTruthy();
    expect(screen.getByText(currentTickStatus("ja", 9, 9))).toBeTruthy();
    expect(currentButton.disabled).toBe(true);

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(screen.getByText(currentTickStatus("ja", 3, 9))).toBeTruthy();
    });
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
      screen.getByRole<HTMLButtonElement>("button", {
        name: messages.returnToCurrentTickLabel,
      }).disabled,
    ).toBe(true);
  });
});
