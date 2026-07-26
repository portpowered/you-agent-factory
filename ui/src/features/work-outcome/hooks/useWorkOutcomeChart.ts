import { useMemo } from "react";
import type { WorkChartState } from "../components/work-chart/work-chart";
import {
  isSupportedMaterializedWorkOutcomeState,
  selectMaterializedWorkOutcomeSamples,
} from "../lib/materializer/materialized-work-outcome";
import { buildWorkChartModel } from "../lib/trends";

const WORK_OUTCOME_RANGE_ID = "session";
const SESSION_WORK_CHART_NOW = 0;

export function useWorkOutcomeChart({
  hydrationStatus = "ready",
  locale,
  materializedWorkOutcomeState,
  selectedTimelineTick,
}: {
  hydrationStatus?: "loading" | "ready";
  locale?: string | null;
  materializedWorkOutcomeState: unknown;
  selectedTimelineTick: number;
}) {
  const workOutcomeSamples = useMemo(() => {
    if (
      hydrationStatus !== "ready" ||
      !isSupportedMaterializedWorkOutcomeState(materializedWorkOutcomeState)
    ) {
      return [];
    }
    return selectMaterializedWorkOutcomeSamples(
      materializedWorkOutcomeState,
      selectedTimelineTick,
    );
  }, [hydrationStatus, materializedWorkOutcomeState, selectedTimelineTick]);

  const chartStatus: WorkChartState["status"] =
    hydrationStatus === "loading"
      ? "loading"
      : isSupportedMaterializedWorkOutcomeState(materializedWorkOutcomeState)
        ? "ready"
        : "error";

  return useMemo(
    () => ({
      ...buildWorkChartModel(
        workOutcomeSamples,
        WORK_OUTCOME_RANGE_ID,
        SESSION_WORK_CHART_NOW,
        locale,
      ),
      chartState: workChartState(chartStatus),
    }),
    [chartStatus, locale, workOutcomeSamples],
  );
}

function workChartState(status: WorkChartState["status"]): WorkChartState {
  switch (status) {
    case "loading":
      return { status: "loading" };
    case "error":
      return { status: "error" };
    default:
      return { status: "ready" };
  }
}
