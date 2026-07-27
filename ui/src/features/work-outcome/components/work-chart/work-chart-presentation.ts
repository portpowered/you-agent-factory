import type { ChartPresentation } from "@you-agent-factory/components/charts";

// tailwind-exception: intrinsic-sizing
const WORK_CHART_READY_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full select-none flex-col px-5 pb-5 pt-4 sm:px-6 sm:pb-6 sm:pt-5";
// tailwind-exception: intrinsic-sizing
const WORK_CHART_EMBEDDED_READY_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full select-none flex-col px-0 pb-4 pt-0";

const WORK_CHART_OVERLAY_CLASS =
  "flex h-full flex-col gap-2 px-5 pb-4 pt-4 sm:px-6 sm:pb-5 sm:pt-5";
const WORK_CHART_EMBEDDED_OVERLAY_CLASS =
  "flex h-full flex-col gap-2 px-0 pb-3 pt-0";

export function workChartPresentationClasses(presentation: ChartPresentation): {
  overlayClassName: string;
  readyClassName: string;
} {
  const embedded = presentation === "embedded";
  return {
    overlayClassName: embedded
      ? WORK_CHART_EMBEDDED_OVERLAY_CLASS
      : WORK_CHART_OVERLAY_CLASS,
    readyClassName: embedded
      ? WORK_CHART_EMBEDDED_READY_CLASS
      : WORK_CHART_READY_CLASS,
  };
}
