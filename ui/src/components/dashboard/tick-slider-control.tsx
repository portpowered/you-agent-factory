import { useMemo, type ChangeEvent } from "react";

import { useFactoryTimelineStore } from "../../state/factoryTimelineStore";
import { DashboardButton } from "./button";
import { cx } from "./classnames";

const TICK_SLIDER_SHELL_CLASS =
  "flex min-w-64 flex-1 flex-wrap items-center gap-3 rounded-lg border border-af-overlay/10 bg-af-overlay/4 px-3 py-2";
const TICK_SLIDER_LABEL_CLASS =
  "flex min-w-36 flex-1 flex-col gap-1 text-xs font-bold uppercase tracking-[0.16em] text-af-ink/62";
const TICK_SLIDER_INPUT_CLASS =
  "h-2 min-w-44 flex-1 cursor-pointer accent-af-accent disabled:cursor-not-allowed disabled:opacity-45";
const TICK_SLIDER_BUTTON_CLASS = "px-3 py-2 text-sm font-bold";
const TICK_SLIDER_STATUS_CLASS = "text-sm text-af-ink/76";
const MINIMUM_TIMELINE_TICKS = 2;

interface TimelineBounds {
  maxTick: number;
  minTick: number;
  tickCount: number;
}

function timelineBounds(eventTicks: number[], cachedTicks: string[], latestTick: number): TimelineBounds {
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

export function TickSliderControl() {
  const eventTicks = useFactoryTimelineStore((state) =>
    state.events.map((event) => event.context.tick),
  );
  const cachedTicks = useFactoryTimelineStore((state) => Object.keys(state.worldViewCache));
  const latestTick = useFactoryTimelineStore((state) => state.latestTick);
  const mode = useFactoryTimelineStore((state) => state.mode);
  const selectTick = useFactoryTimelineStore((state) => state.selectTick);
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const setCurrentMode = useFactoryTimelineStore((state) => state.setCurrentMode);
  const bounds = useMemo(
    () => timelineBounds(eventTicks, cachedTicks, latestTick),
    [eventTicks, cachedTicks, latestTick],
  );
  const isDisabled =
    bounds.tickCount < MINIMUM_TIMELINE_TICKS || bounds.maxTick <= bounds.minTick;
  const displayedTick = Math.min(Math.max(selectedTick, bounds.minTick), bounds.maxTick);

  const handleTickChange = (event: ChangeEvent<HTMLInputElement>) => {
    selectTick(Number(event.target.value));
  };

  return (
    <div className={TICK_SLIDER_SHELL_CLASS}>
      <label className={TICK_SLIDER_LABEL_CLASS}>
        Timeline tick
        <input
          aria-label="Timeline tick"
          className={TICK_SLIDER_INPUT_CLASS}
          disabled={isDisabled}
          max={bounds.maxTick}
          min={bounds.minTick}
          onChange={handleTickChange}
          type="range"
          value={displayedTick}
        />
      </label>

      <span className={TICK_SLIDER_STATUS_CLASS}>
        {isDisabled
          ? "Waiting for more ticks"
          : `Tick ${displayedTick} of ${bounds.maxTick}`}
      </span>

      <DashboardButton
        className={cx(TICK_SLIDER_BUTTON_CLASS, mode === "current" && "opacity-75")}
        disabled={isDisabled || mode === "current"}
        onClick={setCurrentMode}
        tone="secondary"
      >
        Current
      </DashboardButton>
    </div>
  );
}
