import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { graphStateSmokeTimelineEvents } from "../../components/dashboard/fixtures";
import { TickSliderControl } from "./tick-slider-control";
import type { FactoryEvent } from "../../api/events";
import { FACTORY_EVENT_TYPES } from "../../api/events";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";
import { describe, afterEach, it, expect } from "vitest";

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

    render(<TickSliderControl />);

    expect(screen.getByRole<HTMLInputElement>("slider", { name: "Timeline tick" }).disabled).toBe(
      true,
    );
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
    const currentButton = screen.getByRole<HTMLButtonElement>("button", { name: "Current" });

    expect(currentButton.disabled).toBe(true);
    expect(currentButton.className).toContain("bg-af-accent/10");
  });

  it("switches between fixed and current mode through the rendered controls", async () => {
    useFactoryTimelineStore.getState().replaceEvents(graphStateSmokeTimelineEvents);

    render(<TickSliderControl />);

    const slider = screen.getByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    const currentButton = screen.getByRole<HTMLButtonElement>("button", { name: "Current" });

    expect(slider.value).toBe("9");
    expect(screen.getByText("Tick 9 of 9")).toBeTruthy();
    expect(currentButton.disabled).toBe(true);
    expect(currentButton.className).toContain("bg-af-accent/10");
    expect(currentButton.className).toContain("opacity-75");
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
});
