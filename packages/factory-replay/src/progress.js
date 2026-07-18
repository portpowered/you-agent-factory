const SYSTEM_WORK_TYPE_ID = "__system_time";

/**
 * Partition customer Work into one mutually exclusive progress category.
 *
 * @param {import("./index.d.ts").FactoryWorkProgressProjectionInput} input
 * @returns {import("./index.d.ts").FactoryWorkProgressProjection}
 */
export function projectFactoryWorkProgress(input) {
  const statesByWorkType = indexWorkStates(input.factory);
  const activeWorkIds = new Set(input.activeWorkIds);
  const worksById = new Map();
  for (const evidence of input.works) {
    if (!evidence.id || evidence.workTypeId === SYSTEM_WORK_TYPE_ID) {
      continue;
    }
    worksById.set(evidence.id, { ...evidence });
  }

  /** @type {import("./index.d.ts").FactoryWorkProgressProjection} */
  const projection = {
    active: [],
    completed: [],
    counts: { active: 0, completed: 0, failed: 0, queued: 0, unclassified: 0 },
    failed: [],
    queued: [],
    selectedTick: input.selectedTick,
    total: worksById.size,
    unclassified: [],
  };
  for (const evidence of [...worksById.values()].sort((left, right) =>
    left.id.localeCompare(right.id),
  )) {
    const state = resolveState(evidence, statesByWorkType);
    const work = {
      id: evidence.id,
      ...(evidence.workTypeId ? { workTypeId: evidence.workTypeId } : {}),
      ...(state?.id ? { stateId: state.id } : {}),
      ...(state?.name ? { stateName: state.name } : {}),
    };
    const category = classifyWork(state?.category, activeWorkIds.has(evidence.id));
    projection[category].push(work);
    projection.counts[category] += 1;
  }
  return projection;
}

/** @typedef {{id?: string, name: string, type: import("./index.d.ts").FactoryWorkStateCategory}} IndexedWorkState */

/**
 * @param {import("@you-agent-factory/client").FactoryDefinition | undefined} factory
 * @returns {Map<string, Map<string, IndexedWorkState>>}
 */
function indexWorkStates(factory) {
  const statesByWorkType = new Map();
  for (const workType of factory?.workTypes ?? []) {
    const states = new Map();
    for (const state of workType.states) {
      states.set(state.name, state);
      if (state.id?.trim()) {
        states.set(state.id, state);
      }
    }
    statesByWorkType.set(workType.name, states);
    if (workType.id?.trim()) {
      statesByWorkType.set(workType.id, states);
    }
  }
  return statesByWorkType;
}

/**
 * @param {import("./index.d.ts").FactoryWorkProgressEvidence} evidence
 * @param {Map<string, Map<string, IndexedWorkState>>} statesByWorkType
 */
function resolveState(evidence, statesByWorkType) {
  if (evidence.state?.category) {
    return evidence.state;
  }
  if (!evidence.workTypeId) {
    return evidence.state;
  }
  const states = statesByWorkType.get(evidence.workTypeId);
  if (evidence.state?.name) {
    return progressState(states?.get(evidence.state.name)) ?? evidence.state;
  }
  if (evidence.state?.id) {
    return progressState(states?.get(evidence.state.id)) ?? evidence.state;
  }
  return evidence.state;
}

/** @param {IndexedWorkState | undefined} state @returns {import("./index.d.ts").FactoryWorkProgressStateEvidence | undefined} */
function progressState(state) {
  return state
    ? {
        category: state.type,
        ...(state.id ? { id: state.id } : {}),
        name: state.name,
      }
    : undefined;
}

/**
 * @param {import("./index.d.ts").FactoryWorkStateCategory | undefined} stateCategory
 * @param {boolean} active
 * @returns {import("./index.d.ts").FactoryWorkProgressCategory}
 */
function classifyWork(stateCategory, active) {
  if (stateCategory === "FAILED") return "failed";
  if (stateCategory === "TERMINAL") return "completed";
  if (active) return "active";
  if (stateCategory === "INITIAL" || stateCategory === "PROCESSING") {
    return "queued";
  }
  return "unclassified";
}
