import type { ElementType, HTMLAttributes, ReactNode } from "react";

import { AgentBentoCard } from "../../features/bento/components/agent-bento";
import { cn } from "../../lib/cn";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_WIDGET_SUBTITLE_CLASS,
} from "./dashboard-typography";

const DASHBOARD_WIDGET_CLASS = "min-w-0";
const DETAIL_CARD_CLASS = cn(
  "[&_dd]:m-0 [&_dl]:m-0 [&_dl]:grid [&_dl]:gap-3 [&_dl_div:first-child]:border-t-0 [&_dl_div:first-child]:pt-0 [&_dl_div]:border-t [&_dl_div]:border-outline [&_dl_div]:pt-3 [&_dt]:mb-1 [&_h3]:mt-0",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const DETAIL_CARD_WIDE_CLASS = "min-h-72";
const WIDGET_SUBTITLE_CLASS = cn("mt-0", DASHBOARD_WIDGET_SUBTITLE_CLASS);
const DETAIL_COPY_CLASS = cn("m-0", DASHBOARD_BODY_TEXT_CLASS);
const EMPTY_STATE_CLASS =
  "grid min-h-60 items-start gap-1.5 rounded-2xl border border-dashed border-outline-variant bg-surface-container-low p-5 [&_h3]:m-0";
const EMPTY_STATE_COMPACT_CLASS = "min-h-0";

export interface WidgetSubtitleProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  children: ReactNode;
}

export function WidgetSubtitle({
  as: Component = "p",
  children,
  className,
  ...props
}: WidgetSubtitleProps) {
  return (
    <Component className={cn(WIDGET_SUBTITLE_CLASS, className)} {...props}>
      {children}
    </Component>
  );
}

export interface DetailCopyProps extends HTMLAttributes<HTMLParagraphElement> {
  children: ReactNode;
}

export function DetailCopy({ children, className, ...props }: DetailCopyProps) {
  return (
    <p className={cn(DETAIL_COPY_CLASS, className)} {...props}>
      {children}
    </p>
  );
}

export interface DashboardEmptyStateProps
  extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
  compact?: boolean;
}

export function DashboardEmptyState({
  children,
  className,
  compact = false,
  ...props
}: DashboardEmptyStateProps) {
  return (
    <div
      className={cn(
        EMPTY_STATE_CLASS,
        compact && EMPTY_STATE_COMPACT_CLASS,
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

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
