import { startTransition, useEffect, useState } from "react";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "./api/events";

interface Workstation {
  id: string;
  name: string;
}

interface WorkItem {
  id: string;
  name: string;
  traceID: string;
  typeName: string;
}

interface DispatchRecord {
  id: string;
  transitionId: string;
  workIds: string[];
}

interface DashboardSnapshot {
  connectionState: "connecting" | "error" | "open";
  dispatches: DispatchRecord[];
  factoryState: string;
  latestTick: number;
  workItems: Record<string, WorkItem>;
  workstations: Workstation[];
}

const eventStreamPath = "/events";
const initialSnapshot: DashboardSnapshot = {
  connectionState: "connecting",
  dispatches: [],
  factoryState: "Connecting",
  latestTick: 0,
  workItems: {},
  workstations: [],
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function readString(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function readTick(event: FactoryEvent): number {
  if (!isRecord(event.context)) {
    return 0;
  }

  const tick = event.context.tick;
  return typeof tick === "number" ? tick : 0;
}

function extractWorkstations(factory: unknown): Workstation[] {
  if (!isRecord(factory) || !Array.isArray(factory.workstations)) {
    return [];
  }

  return factory.workstations
    .map((candidate) => {
      if (!isRecord(candidate)) {
        return null;
      }

      const id = readString(candidate.id);
      const name = readString(candidate.name);
      if (!id || !name) {
        return null;
      }

      return { id, name };
    })
    .filter((candidate): candidate is Workstation => candidate !== null);
}

function extractWorkItem(candidate: unknown): WorkItem | null {
  if (!isRecord(candidate)) {
    return null;
  }

  const id = readString(candidate.work_id) ?? readString(candidate.workId);
  const name = readString(candidate.name);
  const traceID = readString(candidate.trace_id) ?? readString(candidate.current_chaining_trace_id) ?? "trace-unavailable";
  const typeName = readString(candidate.work_type_name) ?? "unknown";

  if (!id || !name) {
    return null;
  }

  return {
    id,
    name,
    traceID,
    typeName,
  };
}

function mergeWorkItems(snapshot: DashboardSnapshot, candidates: unknown): DashboardSnapshot {
  if (!Array.isArray(candidates)) {
    return snapshot;
  }

  const nextWorkItems = { ...snapshot.workItems };
  let changed = false;

  for (const candidate of candidates) {
    const workItem = extractWorkItem(candidate);
    if (!workItem) {
      continue;
    }

    nextWorkItems[workItem.id] = workItem;
    changed = true;
  }

  return changed ? { ...snapshot, workItems: nextWorkItems } : snapshot;
}

function extractDispatchRecord(event: FactoryEvent, payload: unknown): DispatchRecord | null {
  if (!isRecord(payload)) {
    return null;
  }

  const context = isRecord(event.context) ? event.context : {};
  const dispatchId = readString(payload.dispatchId) ?? readString(context.dispatchId);
  const transitionId = readString(payload.transitionId);
  if (!dispatchId || !transitionId) {
    return null;
  }

  const workIds = new Set<string>();

  if (Array.isArray(payload.inputs)) {
    for (const input of payload.inputs) {
      if (!isRecord(input)) {
        continue;
      }

      const workId = readString(input.workId) ?? readString(input.work_id);
      if (workId) {
        workIds.add(workId);
      }
    }
  }

  if (Array.isArray(context.workIds)) {
    for (const workId of context.workIds) {
      const resolvedWorkId = readString(workId);
      if (resolvedWorkId) {
        workIds.add(resolvedWorkId);
      }
    }
  }

  return {
    id: dispatchId,
    transitionId,
    workIds: [...workIds],
  };
}

function reduceSnapshot(snapshot: DashboardSnapshot, event: FactoryEvent): DashboardSnapshot {
  const payload = isRecord(event.payload) ? event.payload : {};
  let nextSnapshot: DashboardSnapshot = {
    ...snapshot,
    connectionState: "open",
    latestTick: Math.max(snapshot.latestTick, readTick(event)),
  };

  if (event.type === FACTORY_EVENT_TYPES.runRequest || event.type === FACTORY_EVENT_TYPES.initialStructureRequest) {
    const workstations = extractWorkstations(payload.factory);
    if (workstations.length > 0) {
      nextSnapshot = {
        ...nextSnapshot,
        workstations,
      };
    }
  }

  if (event.type === FACTORY_EVENT_TYPES.factoryStateResponse) {
    const factoryState = readString(payload.state);
    if (factoryState) {
      nextSnapshot = {
        ...nextSnapshot,
        factoryState,
      };
    }
  }

  if (event.type === FACTORY_EVENT_TYPES.workRequest) {
    nextSnapshot = mergeWorkItems(nextSnapshot, payload.works);
  }

  if (event.type === FACTORY_EVENT_TYPES.dispatchRequest) {
    const dispatch = extractDispatchRecord(event, payload);
    if (dispatch) {
      nextSnapshot = {
        ...nextSnapshot,
        dispatches: [
          ...nextSnapshot.dispatches.filter((candidate) => candidate.id !== dispatch.id),
          dispatch,
        ],
      };
    }
  }

  if (event.type === FACTORY_EVENT_TYPES.dispatchResponse) {
    nextSnapshot = mergeWorkItems(nextSnapshot, payload.outputWork);
  }

  return nextSnapshot;
}

function resolveEventStreamURL() {
  const apiOrigin = import.meta.env.VITE_AGENT_FACTORY_API_ORIGIN;
  return apiOrigin ? `${apiOrigin}${eventStreamPath}` : eventStreamPath;
}

function workstationButtonLabel(workstation: Workstation): string {
  return `Select ${workstation.name} workstation`;
}

function uniqueWorkItemsForWorkstation(snapshot: DashboardSnapshot, workstationId: string): WorkItem[] {
  const workItems: WorkItem[] = [];
  const seen = new Set<string>();

  for (const dispatch of snapshot.dispatches) {
    if (dispatch.transitionId !== workstationId) {
      continue;
    }

    for (const workId of dispatch.workIds) {
      const workItem = snapshot.workItems[workId];
      if (!workItem || seen.has(workItem.id)) {
        continue;
      }

      seen.add(workItem.id);
      workItems.push(workItem);
    }
  }

  return workItems;
}

export function App() {
  const [snapshot, setSnapshot] = useState(initialSnapshot);
  const [selectedWorkstationId, setSelectedWorkstationId] = useState<string | null>(null);
  const [selectedWorkItemId, setSelectedWorkItemId] = useState<string | null>(null);

  useEffect(() => {
    const eventSource = new EventSource(resolveEventStreamURL());

    eventSource.onopen = () => {
      startTransition(() => {
        setSnapshot((current) => ({
          ...current,
          connectionState: "open",
        }));
      });
    };

    eventSource.onerror = () => {
      startTransition(() => {
        setSnapshot((current) => ({
          ...current,
          connectionState: "error",
        }));
      });
    };

    eventSource.onmessage = (message) => {
      const event = JSON.parse(message.data) as FactoryEvent;
      startTransition(() => {
        setSnapshot((current) => reduceSnapshot(current, event));
      });
    };

    return () => {
      eventSource.close();
    };
  }, []);

  const selectedWorkstation =
    snapshot.workstations.find((workstation) => workstation.id === selectedWorkstationId) ?? null;
  const workstationWorkItems = selectedWorkstation
    ? uniqueWorkItemsForWorkstation(snapshot, selectedWorkstation.id)
    : [];
  const selectedWorkItem =
    workstationWorkItems.find((workItem) => workItem.id === selectedWorkItemId)
    ?? workstationWorkItems[0]
    ?? null;

  return (
    <main className="app-shell">
      <section className="panel panel-hero">
        <div>
          <p className="eyebrow">Agent Factory</p>
          <h1>Agent Factory</h1>
        </div>
        <p className="lede">
          Embedded dashboard smoke surface for the repository-owned replay and CI workflow.
        </p>
        <p className="api-contract">
          Canonical event stream endpoint: <code>{eventStreamPath}</code>
        </p>
      </section>

      <section className="panel summary-grid">
        <article aria-label="dashboard summary" className="summary-card">
          <p className="summary-label">Factory state</p>
          <strong>{snapshot.factoryState}</strong>
        </article>
        <article className="summary-card">
          <p className="summary-label">Replay progress</p>
          <strong>{`Tick ${snapshot.latestTick} of ${snapshot.latestTick}`}</strong>
        </article>
        <article className="summary-card">
          <p className="summary-label">Stream status</p>
          <strong>{snapshot.connectionState}</strong>
        </article>
      </section>

      <section className="panel">
        <div className="section-heading">
          <h2>Workstations</h2>
          <p>Select a workstation to inspect replayed work items.</p>
        </div>
        <div className="button-row">
          {snapshot.workstations.map((workstation) => (
            <button
              className={selectedWorkstationId === workstation.id ? "workstation-button active" : "workstation-button"}
              key={workstation.id}
              onClick={() => {
                setSelectedWorkstationId(workstation.id);
                setSelectedWorkItemId(null);
              }}
              type="button"
            >
              {workstationButtonLabel(workstation)}
            </button>
          ))}
        </div>
      </section>

      <section className="detail-layout">
        <article aria-label="Current selection" className="panel detail-card">
          <h2>Current selection</h2>
          {selectedWorkstation ? (
            <div className="selection-stack">
              <p>
                <span className="detail-label">Workstation</span>
                <strong>{selectedWorkstation.name}</strong>
              </p>
              {selectedWorkItem ? (
                <>
                  <p>
                    <span className="detail-label">Work item</span>
                    <strong>{selectedWorkItem.name}</strong>
                  </p>
                  <p>
                    <span className="detail-label">Work type</span>
                    <strong>{selectedWorkItem.typeName}</strong>
                  </p>
                </>
              ) : (
                <p className="muted-copy">No work items have been replayed for this workstation yet.</p>
              )}
            </div>
          ) : (
            <p className="muted-copy">Choose a workstation to inspect the replayed state.</p>
          )}

          {workstationWorkItems.length > 0 ? (
            <div className="work-item-list">
              {workstationWorkItems.map((workItem, index) => (
                <button
                  className={selectedWorkItemId === workItem.id ? "work-item-button active" : "work-item-button"}
                  key={workItem.id}
                  onClick={() => {
                    setSelectedWorkItemId(workItem.id);
                  }}
                  type="button"
                >
                  {`Select work item ${index + 1}`}
                </button>
              ))}
            </div>
          ) : null}
        </article>

        <article aria-label="Trace drill-down" className="panel detail-card">
          <h2>Trace drill-down</h2>
          {selectedWorkItemId && selectedWorkItem ? (
            <div className="selection-stack">
              <p>
                <span className="detail-label">Trace</span>
                <strong>{selectedWorkItem.traceID}</strong>
              </p>
              <p>
                <span className="detail-label">Work item</span>
                <strong>{selectedWorkItem.name}</strong>
              </p>
              <p className="muted-copy">
                Replay-driven drill-down uses the recorded event stream rather than seeded component props.
              </p>
            </div>
          ) : (
            <p className="muted-copy">Select a replayed work item to inspect its trace context.</p>
          )}
        </article>
      </section>
    </main>
  );
}
