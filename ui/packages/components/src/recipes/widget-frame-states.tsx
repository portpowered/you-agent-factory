import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../utilities/cn";

import { WidgetFrameSkeleton } from "./widget-frame-skeleton";

const WIDGET_STATE_PANEL_CLASS = "grid gap-2";
const WIDGET_ERROR_STATE_CLASS =
  "rounded-2xl border border-af-danger-border bg-error-container p-5 text-on-error-container";
const WIDGET_SUCCESS_STATE_CLASS =
  "rounded-2xl border border-af-success-border bg-success-container p-5 text-on-success-container";
const WIDGET_LOADING_PLACEHOLDER_CLASS = "grid gap-2 pt-2";

export interface WidgetLoadingStateProps
  extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
  placeholder?: ReactNode;
  showDefaultPlaceholder?: boolean;
}

export function WidgetLoadingState({
  children,
  className,
  placeholder,
  showDefaultPlaceholder = true,
  ...props
}: WidgetLoadingStateProps) {
  const resolvedPlaceholder =
    placeholder ??
    (showDefaultPlaceholder ? (
      <>
        <WidgetFrameSkeleton className="h-4 w-full max-w-48" />
        <WidgetFrameSkeleton className="h-24 w-full" />
        <WidgetFrameSkeleton className="h-4 w-full max-w-48" />
      </>
    ) : null);

  return (
    <div
      aria-busy="true"
      className={cn(WIDGET_STATE_PANEL_CLASS, className)}
      role="status"
      {...props}
    >
      {children}
      {resolvedPlaceholder ? (
        <div aria-hidden="true" className={WIDGET_LOADING_PLACEHOLDER_CLASS}>
          {resolvedPlaceholder}
        </div>
      ) : null}
    </div>
  );
}

export interface WidgetErrorStateProps extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
}

export function WidgetErrorState({
  children,
  className,
  ...props
}: WidgetErrorStateProps) {
  return (
    <div
      className={cn(
        WIDGET_ERROR_STATE_CLASS,
        WIDGET_STATE_PANEL_CLASS,
        className,
      )}
      role="alert"
      {...props}
    >
      {children}
    </div>
  );
}

export interface WidgetSuccessStateProps
  extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
}

export function WidgetSuccessState({
  children,
  className,
  ...props
}: WidgetSuccessStateProps) {
  return (
    <div
      className={cn(
        WIDGET_SUCCESS_STATE_CLASS,
        WIDGET_STATE_PANEL_CLASS,
        className,
      )}
      role="status"
      {...props}
    >
      {children}
    </div>
  );
}
