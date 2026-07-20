import type { FactoryEvent } from "@you-agent-factory/client";
import type { FactoryEmulatorOutcome } from "@you-agent-factory/factory-emulator";
import {
  canonicalizeFactoryEvents,
  type FactoryActivityProjection,
  type FactoryLoadProjection,
  type FactoryReplayWorldReducer,
  type FactoryTopologyProjection,
  type FactoryWorkProgressProjection,
  projectFactoryActivityAtTick,
  projectFactoryLoadAtTick,
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgressAtTick,
} from "@you-agent-factory/factory-replay";

import type { CustomerFactoryEmulatorDemoFixture } from "./customer-demo-fixtures";

export interface CustomerFactoryEmulatorDemoWorld {
  readonly activity: FactoryActivityProjection;
  readonly load: FactoryLoadProjection;
  readonly progress: FactoryWorkProgressProjection;
  readonly topology: FactoryTopologyProjection;
}

export interface CustomerFactoryEmulatorActivity {
  readonly activityLabel?: string;
  readonly durationMs?: number;
  readonly workstation: string;
}

export const customerFactoryEmulatorDemoReducer: FactoryReplayWorldReducer<
  FactoryEvent[],
  CustomerFactoryEmulatorDemoWorld
> = {
  applyEvent: (events, event) => [...events, structuredClone(event)],
  createState: () => [],
  projectWorld: (events) => {
    const tick = events.reduce(
      (latest, event) => Math.max(latest, event.context.tick),
      0,
    );
    return {
      activity: projectFactoryActivityAtTick({ events, tick }),
      load: projectFactoryLoadAtTick({ events, tick }),
      progress: projectFactoryWorkProgressAtTick({ events, tick }),
      topology: projectFactoryTopologyAtTick({ events, tick }),
    };
  },
};

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}

function stringField(value: unknown, field: string): string | undefined {
  const candidate = record(value)?.[field];
  return typeof candidate === "string" ? candidate : undefined;
}

function matchingOutcome(
  fixture: CustomerFactoryEmulatorDemoFixture,
  events: readonly FactoryEvent[],
  workstation: string,
): FactoryEmulatorOutcome | undefined {
  const completedAtWorkstation = events.filter(
    (event) =>
      event.type === "DISPATCH_RESPONSE" &&
      stringField(event.payload, "transitionId") === workstation,
  ).length;
  const rule = fixture.scenario.rules.find(
    ({ selector }) => selector.workstation === workstation,
  );
  return rule?.outcomes[completedAtWorkstation];
}

/** Resolve current activity while keeping transient scenario copy out of replay. */
export function selectCustomerFactoryEmulatorActivity(
  fixture: CustomerFactoryEmulatorDemoFixture,
  events: readonly FactoryEvent[],
  selectedTick: number,
  includeTransientLabel: boolean,
): CustomerFactoryEmulatorActivity | undefined {
  const accepted = canonicalizeFactoryEvents(events).filter(
    (event) => event.context.tick <= selectedTick,
  );
  const active = new Map<string, FactoryEvent>();
  for (const event of accepted) {
    const dispatchId = event.context.dispatchId;
    if (!dispatchId) continue;
    if (event.type === "DISPATCH_REQUEST") active.set(dispatchId, event);
    if (event.type === "DISPATCH_RESPONSE") active.delete(dispatchId);
  }
  const request = [...active.values()].at(-1);
  const workstation = stringField(request?.payload, "transitionId");
  if (!request || !workstation) return undefined;

  const outcome = matchingOutcome(fixture, accepted, workstation);
  return {
    ...(includeTransientLabel && outcome?.activityLabel
      ? { activityLabel: outcome.activityLabel }
      : {}),
    ...(outcome ? { durationMs: outcome.durationMs } : {}),
    workstation,
  };
}
