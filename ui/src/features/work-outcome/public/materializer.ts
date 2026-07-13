export {
  compareEventToTimelinePosition,
  type FactoryEventTimelinePosition,
} from "../lib/materializer/factory-event-ordering";

export {
  createMaterializedWorkOutcomeState,
  MATERIALIZED_WORK_OUTCOME_RETENTION,
  MATERIALIZED_WORK_OUTCOME_VERSION,
  type MaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
  retainMaterializedWorkOutcomeState,
} from "../lib/materializer/materialized-work-outcome";
