import type { StreamDerivedCacheIdentity } from "../../../lib/stream-derived-cache-identity";
import type { IndexedDBLike } from "../indexedDBCheckpointRequests";

interface CheckpointWriteLane {
  newestSequence?: number;
  tail: Promise<void>;
}

const writeLanesByDatabase = new WeakMap<
  IndexedDBLike,
  Map<string, CheckpointWriteLane>
>();

function writeAdvancesLane(
  newestSequence: number | undefined,
  candidateSequence: number | undefined,
): boolean {
  if (newestSequence === undefined) {
    return true;
  }
  return candidateSequence !== undefined && candidateSequence > newestSequence;
}

export function enqueueOrderedCheckpointWrite(
  indexedDB: IndexedDBLike,
  streamIdentity: StreamDerivedCacheIdentity,
  afterSequence: number | undefined,
  writeCheckpoint: () => Promise<void>,
): Promise<void> {
  const laneKey = JSON.stringify([
    streamIdentity.backendScopeID,
    streamIdentity.factorySessionID,
    streamIdentity.logicalSessionKeyID,
    streamIdentity.streamGenerationID,
  ]);
  let lanes = writeLanesByDatabase.get(indexedDB);
  if (!lanes) {
    lanes = new Map();
    writeLanesByDatabase.set(indexedDB, lanes);
  }

  let lane = lanes.get(laneKey);
  if (lane && !writeAdvancesLane(lane.newestSequence, afterSequence)) {
    return Promise.resolve();
  }
  const startsLane = !lane;
  if (!lane) {
    lane = { tail: Promise.resolve() };
    lanes.set(laneKey, lane);
  }
  if (afterSequence !== undefined) {
    lane.newestSequence = afterSequence;
  }

  let write: Promise<void>;
  try {
    write = startsLane ? writeCheckpoint() : lane.tail.then(writeCheckpoint);
  } catch (error) {
    lanes.delete(laneKey);
    if (lanes.size === 0) {
      writeLanesByDatabase.delete(indexedDB);
    }
    return Promise.reject(error);
  }
  const settledTail = write.catch(() => {});
  lane.tail = settledTail;
  void settledTail.finally(() => {
    if (lanes.get(laneKey)?.tail !== settledTail) {
      return;
    }
    lanes.delete(laneKey);
    if (lanes.size === 0) {
      writeLanesByDatabase.delete(indexedDB);
    }
  });
  return write;
}
