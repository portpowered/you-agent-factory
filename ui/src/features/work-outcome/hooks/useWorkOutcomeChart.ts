import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryEvent } from "../../../api/events";
import type { WorldState } from "../../timeline/state/factoryTimelineStore";
import {
  createMaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
} from "../lib/materializer/materialized-work-outcome";
import {
  buildWorkChartModel,
  recordThroughputSample,
  type ThroughputSample,
} from "../lib/trends";

const WORK_OUTCOME_RANGE_ID = "session";
const SESSION_WORK_CHART_NOW = 0;

export function useWorkOutcomeChart({
  locale,
  selectedTimelineTick,
  timelineEvents,
  worldViewCache,
}: {
  locale?: string | null;
  selectedTimelineTick: number;
  timelineEvents: FactoryEvent[];
  worldViewCache: Record<number, WorldState | DashboardSnapshot | unknown>;
}) {
  const workOutcomeSamples = useMemo(() => {
    if (timelineEvents.length > 0) {
      return buildWorkOutcomeTimelineSamplesFromEvents(
        timelineEvents,
        selectedTimelineTick,
      );
    }
    return buildWorkOutcomeTimelineSamplesFromCachedSnapshots(
      worldViewCache,
      selectedTimelineTick,
    );
  }, [selectedTimelineTick, timelineEvents, worldViewCache]);

  return useMemo(
    () =>
      buildWorkChartModel(
        workOutcomeSamples,
        WORK_OUTCOME_RANGE_ID,
        SESSION_WORK_CHART_NOW,
        locale,
      ),
    [locale, workOutcomeSamples],
  );
}

export function buildWorkOutcomeTimelineSamplesFromEvents(
  events: FactoryEvent[],
  selectedTick: number,
): ThroughputSample[] {
  const selectedEvents = events.filter(
    (event) => event.context.tick <= selectedTick,
  );
  if (selectedEvents.length === 0) {
    return [];
  }

  return reduceMaterializedWorkOutcomeEvents(
    createMaterializedWorkOutcomeState(),
    selectedEvents,
  ).samples;
}

function buildWorkOutcomeTimelineSamplesFromCachedSnapshots(
  worldViewCache: Record<number, WorldState | DashboardSnapshot | unknown>,
  selectedTick: number,
): ThroughputSample[] {
  const ticks = Object.keys(worldViewCache)
    .map((value) => Number(value))
    .filter((tick) => Number.isFinite(tick) && tick <= selectedTick)
    .sort((left, right) => left - right);

  return ticks.reduce<ThroughputSample[]>((samples, tick, index) => {
    const snapshot = worldViewCache[tick] as DashboardSnapshot | undefined;
    if (!snapshot) {
      return samples;
    }
    return recordThroughputSample(samples, snapshot, index);
  }, []);
}
