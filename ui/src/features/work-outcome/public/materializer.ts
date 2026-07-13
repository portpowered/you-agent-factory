export {
  compareEventToTimelinePosition,
  type FactoryEventTimelinePosition,
} from "../lib/materializer/factory-event-ordering";

export {
  createMaterializedWorkOutcomeState,
  isSupportedMaterializedWorkOutcomeState,
  MATERIALIZED_WORK_OUTCOME_RETENTION,
  MATERIALIZED_WORK_OUTCOME_VERSION,
  type MaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
  retainMaterializedWorkOutcomeState,
  selectMaterializedWorkOutcomeSamples,
} from "../lib/materializer/materialized-work-outcome";
