export {
  compareEventToTimelinePosition,
  type FactoryEventTimelinePosition,
} from "../lib/materializer/factory-event-ordering";

export {
  createMaterializedWorkOutcomeState,
  isSupportedMaterializedWorkOutcomeState,
  MATERIALIZED_WORK_OUTCOME_VERSION,
  type MaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
  selectMaterializedWorkOutcomeSamples,
} from "../lib/materializer/materialized-work-outcome";
