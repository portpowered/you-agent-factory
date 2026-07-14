import type { StreamDerivedCacheIdentity } from "../../../lib/stream-derived-cache-identity";
import type { IndexedDBLike } from "../indexedDBCheckpointRequests";

interface CheckpointWriteLane {
  committedSequence?: number;
  newestSequence?: number;
  tail: Promise<void>;
}

interface CheckpointWriteCoordinator {
  lanes: Map<string, CheckpointWriteLane>;
}

const coordinatorsByDatabase = new WeakMap<
  IndexedDBLike,
  CheckpointWriteCoordinator
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
  writeCheckpoint: () => Promise<boolean>,
): Promise<void> {
  const laneKey = JSON.stringify([
    streamIdentity.backendScopeID,
    streamIdentity.factorySessionID,
    streamIdentity.logicalSessionKeyID,
    streamIdentity.streamGenerationID,
  ]);
  let coordinator = coordinatorsByDatabase.get(indexedDB);
  if (!coordinator) {
    coordinator = {
      lanes: new Map(),
    };
    coordinatorsByDatabase.set(indexedDB, coordinator);
  }

  const { lanes } = coordinator;
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
    const runWrite = async () => {
      if (!writeAdvancesLane(lane.committedSequence, afterSequence)) {
        return;
      }
      if (await writeCheckpoint()) {
        lane.committedSequence = afterSequence;
      }
    };
    write = startsLane ? runWrite() : lane.tail.then(runWrite);
  } catch (error) {
    lanes.delete(laneKey);
    return Promise.reject(error);
  }
  const settledTail = write.catch(() => {});
  lane.tail = settledTail;
  void settledTail.finally(() => {
    if (lanes.get(laneKey)?.tail !== settledTail) {
      return;
    }
    lanes.delete(laneKey);
  });
  return write;
}

export function enqueueOrderedCheckpointClear(
  indexedDB: IndexedDBLike,
  streamIdentity: StreamDerivedCacheIdentity,
  clearCheckpoint: () => Promise<void>,
): Promise<void> {
  const laneKey = JSON.stringify([
    streamIdentity.backendScopeID,
    streamIdentity.factorySessionID,
    streamIdentity.logicalSessionKeyID,
    streamIdentity.streamGenerationID,
  ]);
  let coordinator = coordinatorsByDatabase.get(indexedDB);
  if (!coordinator) {
    coordinator = { lanes: new Map() };
    coordinatorsByDatabase.set(indexedDB, coordinator);
  }

  const { lanes } = coordinator;
  let lane = lanes.get(laneKey);
  const startsLane = !lane;
  if (!lane) {
    lane = { tail: Promise.resolve() };
    lanes.set(laneKey, lane);
  }
  lane.newestSequence = undefined;

  const runClear = async () => {
    await clearCheckpoint();
    lane.committedSequence = undefined;
  };
  const clear = startsLane ? runClear() : lane.tail.then(runClear);
  const settledTail = clear.catch(() => {});
  lane.tail = settledTail;
  void settledTail.finally(() => {
    if (lanes.get(laneKey)?.tail === settledTail) {
      lanes.delete(laneKey);
    }
  });
  return clear;
}
