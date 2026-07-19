export const FACTORY_EMULATOR_SCHEDULER_CANDIDATE_LIMIT = 50;

export interface FactorySchedulerToken {
  readonly tokenId: string;
  readonly customerWork: boolean;
  readonly processing: boolean;
  readonly queuedElapsedMs: number;
  readonly lastDispatchElapsedMs?: number;
}

export interface FactorySchedulerCandidate<Value = unknown> {
  readonly transitionId: string;
  readonly workerId: string;
  readonly workstationKind: "logical" | "normal";
  readonly tokens: readonly FactorySchedulerToken[];
  readonly value: Value;
}

function uniqueTokens(
  candidate: FactorySchedulerCandidate,
): readonly FactorySchedulerToken[] {
  const seen = new Set<string>();
  return candidate.tokens.filter(({ tokenId }) => {
    if (seen.has(tokenId)) return false;
    seen.add(tokenId);
    return true;
  });
}

function earliest(values: readonly (number | undefined)[]): number | undefined {
  const present = values.filter(
    (value): value is number => value !== undefined,
  );
  return present.length === 0 ? undefined : Math.min(...present);
}

function compareOptionalAge(
  left: number | undefined,
  right: number | undefined,
): number {
  if (left === undefined) return right === undefined ? 0 : 1;
  if (right === undefined) return -1;
  return left - right;
}

function compareText(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

function compareTokenIds(
  left: readonly FactorySchedulerToken[],
  right: readonly FactorySchedulerToken[],
): number {
  if (left.length !== right.length) return left.length - right.length;
  for (const [index, token] of left.entries()) {
    const comparison = compareText(token.tokenId, right[index]?.tokenId ?? "");
    if (comparison !== 0) return comparison;
  }
  return 0;
}

/** Go-compatible Work-in-queue ordering for the emulator's supported subset. */
export function compareFactorySchedulerCandidates(
  left: FactorySchedulerCandidate,
  right: FactorySchedulerCandidate,
): number {
  const leftTokens = uniqueTokens(left);
  const rightTokens = uniqueTokens(right);
  const processing =
    rightTokens.filter(({ processing: value }) => value).length -
    leftTokens.filter(({ processing: value }) => value).length;
  if (processing !== 0) return processing;

  const customer =
    Number(rightTokens.some(({ customerWork }) => customerWork)) -
    Number(leftTokens.some(({ customerWork }) => customerWork));
  if (customer !== 0) return customer;

  const workstation =
    Number(left.workstationKind === "normal") -
    Number(right.workstationKind === "normal");
  if (workstation !== 0) return workstation;

  const leftDispatchAge = earliest(
    leftTokens.map(({ lastDispatchElapsedMs }) => lastDispatchElapsedMs),
  );
  const rightDispatchAge = earliest(
    rightTokens.map(({ lastDispatchElapsedMs }) => lastDispatchElapsedMs),
  );
  const initialized =
    Number(rightDispatchAge !== undefined) -
    Number(leftDispatchAge !== undefined);
  if (initialized !== 0) return initialized;
  if (leftDispatchAge !== undefined && rightDispatchAge !== undefined) {
    const dispatchAge = compareOptionalAge(leftDispatchAge, rightDispatchAge);
    if (dispatchAge !== 0) return dispatchAge;
  }

  const queueAge = compareOptionalAge(
    earliest(leftTokens.map(({ queuedElapsedMs }) => queuedElapsedMs)),
    earliest(rightTokens.map(({ queuedElapsedMs }) => queuedElapsedMs)),
  );
  if (queueAge !== 0) return queueAge;

  const transition = compareText(left.transitionId, right.transitionId);
  if (transition !== 0) return transition;
  const worker = compareText(left.workerId, right.workerId);
  if (worker !== 0) return worker;
  return compareTokenIds(leftTokens, rightTokens);
}

/** Selects one deterministic batch while preventing double token consumption. */
export function selectFactorySchedulerCandidates<Value>(
  candidates: readonly FactorySchedulerCandidate<Value>[],
  limit = FACTORY_EMULATOR_SCHEDULER_CANDIDATE_LIMIT,
): readonly FactorySchedulerCandidate<Value>[] {
  const selected: FactorySchedulerCandidate<Value>[] = [];
  const claimed = new Set<string>();
  const boundedLimit = Math.max(
    0,
    Math.min(limit, FACTORY_EMULATOR_SCHEDULER_CANDIDATE_LIMIT),
  );

  for (const candidate of [...candidates].sort(
    compareFactorySchedulerCandidates,
  )) {
    if (selected.length >= boundedLimit) break;
    const tokenIds = uniqueTokens(candidate).map(({ tokenId }) => tokenId);
    if (
      tokenIds.length === 0 ||
      tokenIds.some((tokenId) => claimed.has(tokenId))
    ) {
      continue;
    }
    for (const tokenId of tokenIds) claimed.add(tokenId);
    selected.push(candidate);
  }
  return selected;
}
