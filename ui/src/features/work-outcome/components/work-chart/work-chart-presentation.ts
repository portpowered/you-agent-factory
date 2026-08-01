import type { ChartPresentation } from "@you-agent-factory/components/charts";

// tailwind-exception: intrinsic-sizing
const WORK_CHART_READY_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full select-none flex-col px-5 pb-5 pt-4 sm:px-6 sm:pb-6 sm:pt-5";
// tailwind-exception: intrinsic-sizing
const WORK_CHART_EMBEDDED_READY_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full select-none flex-col px-0 pb-4 pt-0";

export function workChartPresentationClasses(presentation: ChartPresentation): {
  readyClassName: string;
} {
  const embedded = presentation === "embedded";
  return {
    readyClassName: embedded
      ? WORK_CHART_EMBEDDED_READY_CLASS
      : WORK_CHART_READY_CLASS,
  };
}
