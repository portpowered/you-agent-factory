import { useMemo } from "react";

import {
  type MaterializedWorkOutcomeState,
  selectMaterializedWorkOutcomeSamples,
} from "../lib/materializer/materialized-work-outcome";
import { buildWorkChartModel } from "../lib/trends";

const WORK_OUTCOME_RANGE_ID = "session";
const SESSION_WORK_CHART_NOW = 0;

export function useWorkOutcomeChart({
  locale,
  materializedWorkOutcomeState,
  selectedTimelineTick,
}: {
  locale?: string | null;
  materializedWorkOutcomeState: MaterializedWorkOutcomeState;
  selectedTimelineTick: number;
}) {
  const workOutcomeSamples = useMemo(
    () =>
      selectMaterializedWorkOutcomeSamples(
        materializedWorkOutcomeState,
        selectedTimelineTick,
      ),
    [materializedWorkOutcomeState, selectedTimelineTick],
  );

  return useMemo(
    () =>
      buildWorkChartModel(
        workOutcomeSamples,
        WORK_OUTCOME_RANGE_ID,
        SESSION_WORK_CHART_NOW,
        locale,
      ),
    [locale, workOutcomeSamples],
  );
}
