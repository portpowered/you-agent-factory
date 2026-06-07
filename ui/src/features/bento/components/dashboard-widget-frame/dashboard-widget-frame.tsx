import type { HTMLAttributes, ReactNode } from "react";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_WIDGET_SUBTITLE_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import { AgentBentoCard } from "../agent-bento";

const DASHBOARD_WIDGET_CLASS = "min-w-0";
const DETAIL_CARD_CLASS = cn(
  "[&_dd]:m-0 [&_dl]:m-0 [&_dl]:grid [&_dl]:gap-3 [&_dl_div:first-child]:border-t-0 [&_dl_div:first-child]:pt-0 [&_dl_div]:border-t [&_dl_div]:border-outline [&_dl_div]:pt-3 [&_dt]:mb-1 [&_h3]:mt-0",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const DETAIL_CARD_WIDE_CLASS = "min-h-72";

export interface DashboardWidgetFrameProps {
  bodyClassName?: string;
  bodyProps?: HTMLAttributes<HTMLDivElement>;
  bodyScroll?: boolean;
  children: ReactNode;
  className?: string;
  headerAction?: ReactNode;
  title: string;
  wide?: boolean;
  widgetId: string;
}

export function DashboardWidgetFrame({
  bodyClassName,
  bodyProps,
  bodyScroll,
  children,
  className = "",
  headerAction,
  title,
  wide = false,
}: DashboardWidgetFrameProps) {
  return (
    <AgentBentoCard
      className={cn(
        DASHBOARD_WIDGET_CLASS,
        DETAIL_CARD_CLASS,
        wide && DETAIL_CARD_WIDE_CLASS,
        className,
      )}
      bodyClassName={bodyClassName}
      bodyProps={bodyProps}
      bodyScroll={bodyScroll}
      headerAction={headerAction}
      title={title}
    >
      {children}
    </AgentBentoCard>
  );
}
