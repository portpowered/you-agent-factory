import type { ElementType, HTMLAttributes, ReactNode } from "react";

import { cn } from "../utilities/cn";

import {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
  WIDGET_FRAME_SUBTITLE_CLASS,
} from "./widget-frame-typography";

const WIDGET_SUBTITLE_CLASS = cn("mt-0", WIDGET_FRAME_SUBTITLE_CLASS);
const DETAIL_COPY_CLASS = cn("m-0", WIDGET_FRAME_BODY_TEXT_CLASS);
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

export interface WidgetDetailCopyProps
  extends HTMLAttributes<HTMLParagraphElement> {
  children: ReactNode;
}

export function WidgetDetailCopy({
  children,
  className,
  ...props
}: WidgetDetailCopyProps) {
  return (
    <p className={cn(DETAIL_COPY_CLASS, className)} {...props}>
      {children}
    </p>
  );
}

export interface WidgetEmptyStateProps extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
  compact?: boolean;
}

export function WidgetEmptyState({
  children,
  className,
  compact = false,
  ...props
}: WidgetEmptyStateProps) {
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

export interface WidgetEmptyStateTitleProps
  extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  children: ReactNode;
}

export function WidgetEmptyStateTitle({
  as: Component = "h3",
  children,
  className,
  ...props
}: WidgetEmptyStateTitleProps) {
  return (
    <Component
      className={cn(WIDGET_FRAME_SECTION_HEADING_CLASS, className)}
      {...props}
    >
      {children}
    </Component>
  );
}

export interface WidgetEmptyStateTextProps
  extends HTMLAttributes<HTMLParagraphElement> {
  children: ReactNode;
}

export function WidgetEmptyStateText({
  children,
  className,
  ...props
}: WidgetEmptyStateTextProps) {
  return (
    <p
      className={cn("m-0", WIDGET_FRAME_BODY_TEXT_CLASS, className)}
      {...props}
    >
      {children}
    </p>
  );
}
