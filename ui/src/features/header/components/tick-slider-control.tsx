import { type ChangeEvent, useId, useMemo } from "react";
import { cn } from "../../../lib/cn";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import {
  getHeaderControlsMessages,
  HEADER_CURRENT_TICK_TOKEN,
  HEADER_MAX_TICK_TOKEN,
} from "../messages/header-controls";

const MINIMUM_TIMELINE_TICKS = 2;

interface TimelineBounds {
  maxTick: number;
  minTick: number;
  tickCount: number;
}

export interface TickSliderControlProps {
  locale?: string;
}

function timelineBounds(
  eventTicks: number[],
  cachedTicks: string[],
  latestTick: number,
): TimelineBounds {
  const ticks = new Set<number>();
  for (const tick of eventTicks) {
    ticks.add(tick);
  }
  for (const tick of cachedTicks) {
    const numericTick = Number(tick);
    if (Number.isFinite(numericTick)) {
      ticks.add(numericTick);
    }
  }
  if (latestTick > 0) {
    ticks.add(latestTick);
  }

  const orderedTicks = [...ticks].sort((left, right) => left - right);
  const minTick = orderedTicks[0] ?? 0;
  const maxTick = Math.max(latestTick, orderedTicks.at(-1) ?? 0);

  return {
    maxTick,
    minTick,
    tickCount: orderedTicks.length,
  };
}

function formatCurrentTickStatus(
  template: string,
  currentTick: number,
  maxTick: number,
): string {
  return template
    .replaceAll(HEADER_CURRENT_TICK_TOKEN, String(currentTick))
    .replaceAll(HEADER_MAX_TICK_TOKEN, String(maxTick));
}

export function TickSliderControl({ locale }: TickSliderControlProps) {
  const tickStatusID = useId();
  const eventTicks = useFactoryTimelineStore((state) =>
    state.events.map((event) => event.context.tick),
  );
  const cachedTicks = useFactoryTimelineStore((state) =>
    Object.keys(state.worldViewCache),
  );
  const latestTick = useFactoryTimelineStore((state) => state.latestTick);
  const selectTick = useFactoryTimelineStore((state) => state.selectTick);
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const setCurrentMode = useFactoryTimelineStore(
    (state) => state.setCurrentMode,
  );
  const bounds = useMemo(
    () => timelineBounds(eventTicks, cachedTicks, latestTick),
    [eventTicks, cachedTicks, latestTick],
  );
  const isDisabled =
    bounds.tickCount < MINIMUM_TIMELINE_TICKS ||
    bounds.maxTick <= bounds.minTick;
  const displayedTick = Math.min(
    Math.max(selectedTick, bounds.minTick),
    bounds.maxTick,
  );
  const messages = getHeaderControlsMessages(locale);
  const sliderValueText = isDisabled
    ? messages.waitingForMoreTicks
    : formatCurrentTickStatus(
        messages.currentTickStatusTemplate,
        displayedTick,
        bounds.maxTick,
      );

  const handleTickChange = (event: ChangeEvent<HTMLInputElement>) => {
    const nextTick = Number(event.target.value);
    if (nextTick >= bounds.maxTick) {
      setCurrentMode();
      return;
    }
    selectTick(nextTick);
  };

  return (
    <div
      className={cn(
        "flex min-w-0 w-full flex-wrap items-center gap-1.5 px-1 py-1",
        "md:flex-nowrap",
      )}
    >
      <label className="flex min-h-10 min-w-36 flex-1 flex-col justify-center gap-0.5 text-[0.7rem] font-bold uppercase tracking-[0.14em] text-on-surface-subtle md:min-w-52">
        <span className="sr-only">{messages.sliderLabel}</span>
        <input
          aria-describedby={tickStatusID}
          aria-label={messages.sliderAriaLabel}
          aria-valuetext={sliderValueText}
          className={cn(
            "h-10 min-w-32 flex-1 cursor-pointer appearance-none bg-transparent disabled:cursor-not-allowed",
            // semantic-color-exception: system-integration
            "[&::-webkit-slider-runnable-track]:h-1.5 [&::-webkit-slider-runnable-track]:rounded-full [&::-webkit-slider-runnable-track]:[background:linear-gradient(90deg,rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.12),rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.2))]",
            // semantic-color-exception: system-integration
            "[&::-webkit-slider-thumb]:-mt-1.5 [&::-webkit-slider-thumb]:h-4 [&::-webkit-slider-thumb]:w-4 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:border [&::-webkit-slider-thumb]:border-[rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.2)] [&::-webkit-slider-thumb]:bg-[rgb(from_var(--color-af-foundation-surface)_r_g_b_/_0.98)] [&::-webkit-slider-thumb]:[box-shadow:0_0_0_1px_rgb(from_var(--color-af-shadow)_r_g_b_/_0.12)]",
            // semantic-color-exception: system-integration
            "[&::-moz-range-track]:h-1.5 [&::-moz-range-track]:rounded-full [&::-moz-range-track]:border-0 [&::-moz-range-track]:[background:linear-gradient(90deg,rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.12),rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.2))]",
            // semantic-color-exception: system-integration
            "[&::-moz-range-thumb]:h-4 [&::-moz-range-thumb]:w-4 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border [&::-moz-range-thumb]:border-[rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.2)] [&::-moz-range-thumb]:bg-[rgb(from_var(--color-af-foundation-surface)_r_g_b_/_0.98)] [&::-moz-range-thumb]:[box-shadow:0_0_0_1px_rgb(from_var(--color-af-shadow)_r_g_b_/_0.12)]",
            // semantic-color-exception: system-integration
            "[&:focus-visible::-webkit-slider-thumb]:[box-shadow:0_0_0_1px_rgb(from_var(--color-af-shadow)_r_g_b_/_0.12),0_0_0_4px_var(--color-af-focus-ring)] [&:focus-visible::-moz-range-thumb]:[box-shadow:0_0_0_1px_rgb(from_var(--color-af-shadow)_r_g_b_/_0.12),0_0_0_4px_var(--color-af-focus-ring)]",
            // semantic-color-exception: system-integration
            "[&:disabled::-webkit-slider-runnable-track]:[background:rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.08)] [&:disabled::-moz-range-track]:[background:rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.08)]",
            // semantic-color-exception: system-integration
            "[&:disabled::-webkit-slider-thumb]:border-[rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.12)] [&:disabled::-webkit-slider-thumb]:bg-[rgb(from_var(--color-af-foundation-surface)_r_g_b_/_0.72)] [&:disabled::-webkit-slider-thumb]:shadow-none",
            // semantic-color-exception: system-integration
            "[&:disabled::-moz-range-thumb]:border-[rgb(from_var(--color-af-foundation-overlay)_r_g_b_/_0.12)] [&:disabled::-moz-range-thumb]:bg-[rgb(from_var(--color-af-foundation-surface)_r_g_b_/_0.72)] [&:disabled::-moz-range-thumb]:shadow-none",
          )}
          disabled={isDisabled}
          max={bounds.maxTick}
          min={bounds.minTick}
          onChange={handleTickChange}
          type="range"
          value={displayedTick}
        />
      </label>

      <div className="ml-auto flex items-center">
        <output
          className="whitespace-nowrap text-xs font-medium tabular-nums text-on-surface-variant"
          id={tickStatusID}
        >
          {sliderValueText}
        </output>
      </div>
    </div>
  );
}
