import type { FactoryEvent } from "@you-agent-factory/client";
import type { FactoryEmulatorNormalizedRelation } from "./session-contracts.js";

export interface FactoryEmulatorReplayedWork {
  readonly submissionId: string;
  readonly workId: string;
  readonly requestId?: string;
  readonly traceId?: string;
  readonly workType?: string;
  readonly state?: string;
  readonly input?: unknown;
  readonly relations?: readonly FactoryEmulatorNormalizedRelation[];
}

type MutableReplayedWork = Omit<FactoryEmulatorReplayedWork, "relations"> & {
  relations: FactoryEmulatorNormalizedRelation[];
};

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function normalizedRelation(
  value: unknown,
  worksByName: ReadonlyMap<string, MutableReplayedWork>,
): FactoryEmulatorNormalizedRelation | undefined {
  const relation = record(value);
  if (
    relation?.type !== "DEPENDS_ON" ||
    typeof relation.sourceWorkName !== "string" ||
    typeof relation.targetWorkName !== "string"
  ) {
    return undefined;
  }
  const targetWorkId =
    typeof relation.targetWorkId === "string"
      ? relation.targetWorkId
      : worksByName.get(relation.targetWorkName)?.workId;
  if (targetWorkId === undefined) return undefined;
  return {
    type: "DEPENDS_ON",
    sourceWorkName: relation.sourceWorkName,
    targetWorkName: relation.targetWorkName,
    targetWorkId,
    requiredState:
      typeof relation.requiredState === "string"
        ? relation.requiredState
        : "complete",
  };
}

function applyRelation(
  value: unknown,
  worksByName: ReadonlyMap<string, MutableReplayedWork>,
): void {
  const relation = normalizedRelation(value, worksByName);
  if (relation === undefined) return;
  const source = worksByName.get(relation.sourceWorkName);
  if (source === undefined) return;
  const key = (candidate: FactoryEmulatorNormalizedRelation) =>
    `${candidate.type}\u0000${candidate.sourceWorkName}\u0000${candidate.targetWorkId}\u0000${candidate.requiredState}`;
  const existing = source.relations.findIndex(
    (candidate) => key(candidate) === key(relation),
  );
  if (existing === -1) source.relations.push(relation);
  else source.relations[existing] = relation;
}

/** Reapply canonical submission events into detached Work and relationship state. */
export function replayFactoryEmulatorSubmissions(
  events: readonly FactoryEvent[],
): readonly FactoryEmulatorReplayedWork[] {
  const works: MutableReplayedWork[] = [];
  const worksByName = new Map<string, MutableReplayedWork>();
  for (const event of events) {
    const payload = record(event.payload);
    if (event.type === "WORK_REQUEST") {
      const eventWorks = Array.isArray(payload?.works) ? payload.works : [];
      for (const [index, value] of eventWorks.entries()) {
        const work = record(value);
        if (typeof work?.name !== "string" || typeof work.workId !== "string") {
          continue;
        }
        const state = record(work.state);
        const replayed: MutableReplayedWork = {
          submissionId: work.name,
          workId: work.workId,
          ...(typeof work.requestId === "string"
            ? { requestId: work.requestId }
            : event.context.requestId
              ? { requestId: event.context.requestId }
              : {}),
          ...(typeof work.traceId === "string"
            ? { traceId: work.traceId }
            : event.context.traceIds?.[index]
              ? { traceId: event.context.traceIds[index] }
              : {}),
          ...(typeof work.workTypeName === "string"
            ? { workType: work.workTypeName }
            : {}),
          ...(typeof state?.name === "string" ? { state: state.name } : {}),
          ...("payload" in work ? { input: work.payload } : {}),
          relations: [],
        };
        works.push(replayed);
        worksByName.set(replayed.submissionId, replayed);
      }
      for (const relation of Array.isArray(payload?.relations)
        ? payload.relations
        : []) {
        applyRelation(relation, worksByName);
      }
      continue;
    }
    if (event.type === "RELATIONSHIP_CHANGE_REQUEST") {
      applyRelation(record(payload?.relation), worksByName);
    }
  }
  return works.map(({ relations, ...work }) => ({
    ...work,
    ...(relations.length === 0
      ? {}
      : { relations: relations.map((relation) => ({ ...relation })) }),
  }));
}
