import { type ChangeEvent, useMemo } from "react";
import { cx } from "../../lib/cx";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";
import {
  getHeaderControlsMessages,
  HEADER_CURRENT_TICK_TOKEN,
  HEADER_MAX_TICK_TOKEN,
} from "./messages/header-controls";

const TICK_SLIDER_SHELL_CLASS = cx(
  "flex min-w-0 w-full flex-wrap items-center gap-3 rounded-lg border border-af-overlay/10 bg-af-overlay/4 px-3 py-2",
  "min-[721px]:w-auto min-[721px]:min-w-[22rem] min-[721px]:max-w-xl",
);
const TICK_SLIDER_LABEL_CLASS =
  "flex min-w-36 flex-1 flex-col gap-1 text-xs font-bold uppercase tracking-[0.16em] text-af-ink/62";
const TICK_SLIDER_INPUT_CLASS =
  "h-2 min-w-44 flex-1 cursor-pointer accent-af-accent disabled:cursor-not-allowed disabled:opacity-45";
const TICK_SLIDER_STATUS_CLASS = "text-sm text-af-ink/76";
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
  const eventTicks = useFactoryTimelineStore((state) =>
    state.events.map((event) => event.context.tick),
  );
  const cachedTicks = useFactoryTimelineStore((state) =>
    Object.keys(state.worldViewCache),
  );
  const latestTick = useFactoryTimelineStore((state) => state.latestTick);
  const mode = useFactoryTimelineStore((state) => state.mode);
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

  const handleTickChange = (event: ChangeEvent<HTMLInputElement>) => {
    selectTick(Number(event.target.value));
  };

  return (
    <div className={TICK_SLIDER_SHELL_CLASS}>
      <label className={TICK_SLIDER_LABEL_CLASS}>
        {messages.sliderLabel}
        <input
          aria-label={messages.sliderAriaLabel}
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
          ? messages.waitingForMoreTicks
          : formatCurrentTickStatus(
              messages.currentTickStatusTemplate,
              displayedTick,
              bounds.maxTick,
            )}
      </span>

      <DashboardHeaderActionButton
        className={cx(mode === "current" && "opacity-75")}
        aria-label={messages.returnToCurrentTickLabel}
        disabled={isDisabled || mode === "current"}
        onClick={setCurrentMode}
      >
        <svg
          aria-hidden="true"
          fill="none"
          height="18"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="1.8"
          viewBox="0 0 24 24"
          width="18"
        >
          <path d="M6 5.75v12.5" />
          <path d="m10 8.25 8 3.75-8 3.75v-7.5" />
        </svg>
      </DashboardHeaderActionButton>
    </div>
  );
}
