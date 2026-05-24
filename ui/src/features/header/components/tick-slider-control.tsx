import { type ChangeEvent, useId, useMemo } from "react";
import { cn } from "../../../lib/cn";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import {
  getHeaderControlsMessages,
  HEADER_CURRENT_TICK_TOKEN,
  HEADER_MAX_TICK_TOKEN,
} from "../messages/header-controls";

const TICK_SLIDER_SHELL_CLASS = cn(
  "flex min-w-0 w-full flex-wrap items-center gap-1.5 px-1 py-1",
  "md:flex-nowrap",
);
const TICK_SLIDER_LABEL_CLASS =
  "flex min-w-36 flex-1 flex-col gap-0.5 text-[0.7rem] font-bold uppercase tracking-[0.14em] text-af-ink/62 md:min-w-52";
const TICK_SLIDER_INPUT_CLASS =
  "h-1.5 min-w-32 flex-1 cursor-pointer accent-af-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25 disabled:cursor-not-allowed disabled:opacity-45";
const TICK_SLIDER_META_CLASS =
  "ml-auto flex items-center";
const TICK_SLIDER_STATUS_CLASS =
  "whitespace-nowrap text-xs font-medium tabular-nums text-af-ink/76";
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
    <div className={TICK_SLIDER_SHELL_CLASS}>
      <label className={TICK_SLIDER_LABEL_CLASS}>
        <span className="sr-only">{messages.sliderLabel}</span>
        <input
          aria-describedby={tickStatusID}
          aria-label={messages.sliderAriaLabel}
          aria-valuetext={sliderValueText}
          className={TICK_SLIDER_INPUT_CLASS}
          disabled={isDisabled}
          max={bounds.maxTick}
          min={bounds.minTick}
          onChange={handleTickChange}
          type="range"
          value={displayedTick}
        />
      </label>

      <div className={TICK_SLIDER_META_CLASS}>
        <output className={TICK_SLIDER_STATUS_CLASS} id={tickStatusID}>
          {sliderValueText}
        </output>
      </div>
    </div>
  );
}
