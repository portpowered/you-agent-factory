import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../utilities/cn";

import type { ChartPresentation } from "./chart";

export type ChartStateStatus = "empty" | "error" | "loading" | "success";

export interface ChartStatePanelProps extends HTMLAttributes<HTMLDivElement> {
  action?: ReactNode;
  description: string;
  presentation?: ChartPresentation;
  status: ChartStateStatus;
  title: string;
}

// tailwind-exception: intrinsic-sizing
const CHART_STATE_PANEL_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full flex-1 flex-col justify-center";
// tailwind-exception: intrinsic-sizing
const CHART_STATE_PANEL_EMBEDDED_CLASS =
  "grid min-h-[14rem] min-w-0 w-full flex-1 flex-col items-start justify-center gap-1.5 p-0 [&_h3]:m-0";
const CHART_STATE_PANEL_STANDALONE_SHELL_CLASS =
  "grid min-h-60 items-start gap-1.5 rounded-2xl border border-dashed border-outline-variant bg-surface-container-low p-5 [&_h3]:m-0";
const CHART_STATE_PANEL_TITLE_CLASS =
  "m-0 text-title-medium font-semibold text-on-surface";
const CHART_STATE_PANEL_DESCRIPTION_CLASS =
  "m-0 text-body-medium text-on-surface-variant";
const CHART_STATE_PANEL_SKELETON_CLASS =
  "animate-pulse rounded-xl bg-af-overlay";

function chartStateRole(status: ChartStateStatus): "alert" | "status" {
  return status === "error" ? "alert" : "status";
}

function chartStateLive(status: ChartStateStatus): "assertive" | "polite" {
  return status === "error" ? "assertive" : "polite";
}

export function ChartStatePanel({
  action,
  className,
  description,
  presentation = "standalone",
  status,
  title,
  ...props
}: ChartStatePanelProps) {
  const embedded = presentation === "embedded";
  const role = chartStateRole(status);
  const ariaLive = chartStateLive(status);
  const loading = status === "loading";

  const content = (
    <>
      {loading ? (
        <div aria-hidden="true" className="grid w-full gap-3">
          <div className={cn(CHART_STATE_PANEL_SKELETON_CLASS, "h-4 w-32")} />
          <div
            className={cn(CHART_STATE_PANEL_SKELETON_CLASS, "h-28 w-full")}
          />
        </div>
      ) : null}
      <h3 className={CHART_STATE_PANEL_TITLE_CLASS}>{title}</h3>
      <p className={CHART_STATE_PANEL_DESCRIPTION_CLASS}>{description}</p>
      {action ? <div className="mt-2">{action}</div> : null}
    </>
  );

  if (!embedded) {
    return (
      <div
        aria-busy={loading || undefined}
        aria-live={ariaLive}
        className={cn(
          CHART_STATE_PANEL_STANDALONE_SHELL_CLASS,
          CHART_STATE_PANEL_CLASS,
          className,
        )}
        data-chart-presentation={presentation}
        data-chart-state={status}
        role={role}
        {...props}
      >
        {content}
      </div>
    );
  }

  return (
    <div
      aria-busy={loading || undefined}
      aria-live={ariaLive}
      className={cn(
        CHART_STATE_PANEL_EMBEDDED_CLASS,
        CHART_STATE_PANEL_CLASS,
        className,
      )}
      data-chart-presentation={presentation}
      data-chart-state={status}
      role={role}
      {...props}
    >
      {content}
    </div>
  );
}
