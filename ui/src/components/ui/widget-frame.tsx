import type { ReactNode } from "react";

import { AgentBentoCard } from "../../features/bento/agent-bento";
import { cx } from "./classnames";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_WIDGET_SUBTITLE_CLASS,
} from "./dashboard-typography";

export const DASHBOARD_WIDGET_CLASS = "min-w-0";
export const DETAIL_CARD_CLASS = cx(
  "rounded-2xl border-af-overlay/8 bg-af-overlay/4 [&_dd]:m-0 [&_dl]:m-0 [&_dl]:grid [&_dl]:gap-[0.8rem] [&_dl_div:first-child]:border-t-0 [&_dl_div:first-child]:pt-0 [&_dl_div]:border-t [&_dl_div]:border-af-overlay/6 [&_dl_div]:pt-3 [&_dt]:mb-1 [&_h3]:mt-0",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
export const DETAIL_CARD_WIDE_CLASS = "min-h-72";
export const WIDGET_SUBTITLE_CLASS = cx("mt-0", DASHBOARD_WIDGET_SUBTITLE_CLASS);
export const DETAIL_COPY_CLASS = cx("m-0", DASHBOARD_BODY_TEXT_CLASS);
export const EMPTY_STATE_CLASS =
  "grid min-h-60 items-start gap-[0.35rem] rounded-2xl border border-dashed border-af-overlay/15 bg-af-overlay/4 p-5 [&_h3]:m-0";
export const EMPTY_STATE_COMPACT_CLASS = "min-h-0";

export interface DashboardWidgetFrameProps {
  children: ReactNode;
  className?: string;
  headerAction?: ReactNode;
  title: string;
  widgetId: string;
}

export function DashboardWidgetFrame({
  children,
  className = "",
  headerAction,
  title,
}: DashboardWidgetFrameProps) {
  return (
    <AgentBentoCard
      className={cx(DASHBOARD_WIDGET_CLASS, DETAIL_CARD_CLASS, className)}
      headerAction={headerAction}
      title={title}
    >
      {children}
    </AgentBentoCard>
  );
}

