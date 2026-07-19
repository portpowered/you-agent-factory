import type { FactoryActivityProjection } from "@you-agent-factory/factory-replay";

const VISIBLE_ACTIVE_WORK_ROWS = 3;

export interface FactoryTopologyActiveWorkRow {
  durationTicks: number;
  id: string;
}

export interface FactoryTopologyActiveWorkSummary {
  overflowCount: number;
  rows: readonly FactoryTopologyActiveWorkRow[];
}

/**
 * Derives a bounded, read-only activity summary from the prepared projection.
 * A Work can appear in more than one active Dispatch, so retain its longest
 * visible duration rather than duplicating its row.
 */
export function projectFactoryTopologyActiveWork(
  activity: FactoryActivityProjection,
): FactoryTopologyActiveWorkSummary {
  const durationByWorkId = new Map<string, number>();
  for (const overlay of activity.activeDispatchOverlays) {
    const durationTicks = Math.max(
      0,
      activity.selectedTick - overlay.startedTick,
    );
    for (const workId of overlay.workIds ?? []) {
      const previousDuration = durationByWorkId.get(workId);
      if (previousDuration === undefined || durationTicks > previousDuration)
        durationByWorkId.set(workId, durationTicks);
    }
  }

  const rows = [...durationByWorkId]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([id, durationTicks]) => ({ durationTicks, id }));
  return {
    overflowCount: Math.max(0, rows.length - VISIBLE_ACTIVE_WORK_ROWS),
    rows: rows.slice(0, VISIBLE_ACTIVE_WORK_ROWS),
  };
}
