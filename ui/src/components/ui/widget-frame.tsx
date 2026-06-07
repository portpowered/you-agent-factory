import type { ElementType, HTMLAttributes, ReactNode } from "react";

import { cn } from "../../lib/cn";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_WIDGET_SUBTITLE_CLASS,
} from "./dashboard-typography";

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

export interface DashboardEmptyStateTitleProps
  extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  children: ReactNode;
}

export function DashboardEmptyStateTitle({
  as: Component = "h3",
  children,
  className,
  ...props
}: DashboardEmptyStateTitleProps) {
  return (
    <Component
      className={cn(DASHBOARD_SECTION_HEADING_CLASS, className)}
      {...props}
    >
      {children}
    </Component>
  );
}

export interface DashboardEmptyStateTextProps
  extends HTMLAttributes<HTMLParagraphElement> {
  children: ReactNode;
}

export function DashboardEmptyStateText({
  children,
  className,
  ...props
}: DashboardEmptyStateTextProps) {
  return (
    <p className={cn("m-0", DASHBOARD_BODY_TEXT_CLASS, className)} {...props}>
      {children}
    </p>
  );
}

