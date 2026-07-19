import type { FactoryActivityProjection } from "@you-agent-factory/factory-replay";

import type { FactoryTopologyReplayMessages } from "./factory-topology-replay";

export interface ActiveWorkItem {
  durationTicks: number;
  id: string;
}

const VISIBLE_ACTIVE_WORK_LIMIT = 3;

export function ActiveWorkRows({
  items,
  messages,
}: {
  items: readonly ActiveWorkItem[];
  messages: FactoryTopologyReplayMessages;
}) {
  const visibleItems = items.slice(0, VISIBLE_ACTIVE_WORK_LIMIT);
  const overflow = items.length - visibleItems.length;
  return (
    <fieldset
      aria-label={messages.activeWorkRows(items.length)}
      className="factory-topology-replay__active-work"
    >
      <ul className="factory-topology-replay__active-work-list">
        {visibleItems.map((item) => (
          <li
            className="factory-topology-replay__active-work-row"
            key={item.id}
          >
            <span className="factory-topology-replay__active-work-label">
              {item.id}
            </span>
            <span className="factory-topology-replay__active-work-duration">
              {messages.activeWorkDuration(item.durationTicks)}
            </span>
          </li>
        ))}
      </ul>
      {overflow > 0 ? (
        <span className="factory-topology-replay__active-work-overflow">
          {messages.activeWorkOverflow(overflow)}
        </span>
      ) : null}
    </fieldset>
  );
}

export function activeWorkByWorkstationNode(
  activity: FactoryActivityProjection,
): Map<string, ActiveWorkItem[]> {
  const workByWorkstation = new Map<string, ActiveWorkItem[]>();
  for (const dispatch of activity.activeDispatchOverlays) {
    if (!dispatch.workstationNodeId) continue;
    const items = workByWorkstation.get(dispatch.workstationNodeId) ?? [];
    for (const workId of dispatch.workIds ?? []) {
      items.push({
        durationTicks: Math.max(
          0,
          activity.selectedTick - dispatch.startedTick,
        ),
        id: workId,
      });
    }
    workByWorkstation.set(dispatch.workstationNodeId, items);
  }
  for (const items of workByWorkstation.values()) {
    items.sort((left, right) => left.id.localeCompare(right.id));
  }
  return workByWorkstation;
}
