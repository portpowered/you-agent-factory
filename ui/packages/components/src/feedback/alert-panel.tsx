import {
  type ElementType,
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { SurfacePanel, type SurfacePanelProps } from "../layout/surface-panel";
import { cn } from "../utilities/cn";
import {
  type AlertPanelSemanticVariant,
  resolveAlertPanelSemantic,
} from "./alert-panel-semantics";
import { Skeleton } from "./skeleton";

export type AlertPanelTone =
  | "danger"
  | "error"
  | "info"
  | "neutral"
  | "success"
  | "warning";
export type AlertPanelVariant = "default" | "empty" | "loading";

export interface AlertPanelProps
  extends Omit<SurfacePanelProps, "surface" | "tone"> {
  children?: ReactNode;
  compact?: boolean;
  semantic?: AlertPanelSemanticVariant;
  showStatusLabel?: boolean;
  tone?: AlertPanelTone;
  variant?: AlertPanelVariant;
}

const ALERT_PANEL_TONE_CLASS: Record<AlertPanelTone, string> = {
  danger: "border-af-danger-border bg-error-container text-on-error-container",
  error: "border-af-danger-border bg-error-container text-on-error-container",
  info: "border-af-info-border bg-info-container text-on-info-container",
  neutral: "border-outline bg-surface-container-low text-on-surface-variant",
  success:
    "border-af-success-border bg-success-container text-on-success-container",
  warning:
    "border-af-warning-border bg-warning-container text-on-warning-container",
};

const ALERT_PANEL_VARIANT_CLASS: Record<AlertPanelVariant, string> = {
  default: "grid gap-2",
  empty: "grid items-start gap-1.5 rounded-2xl border-dashed p-5 [&_h3]:m-0",
  loading: "grid gap-2",
};
const ALERT_PANEL_COMPACT_CLASS: Record<AlertPanelVariant, string> = {
  default: "",
  empty: "min-h-0",
  loading: "",
};

const ALERT_PANEL_BODY_TEXT_CLASS = "text-body-medium text-on-surface-variant";
const ALERT_PANEL_STATUS_LABEL_CLASS =
  "text-body-small font-medium uppercase tracking-wide text-current";

export const AlertPanel = forwardRef<HTMLDivElement, AlertPanelProps>(
  function AlertPanel(
    {
      "aria-busy": ariaBusy,
      children,
      className,
      compact = false,
      padding = "default",
      radius = "xl",
      role,
      semantic,
      showStatusLabel,
      tone: toneProp = "warning",
      variant: variantProp = "default",
      ...props
    },
    ref,
  ) {
    const semanticConfig = semantic
      ? resolveAlertPanelSemantic(semantic)
      : undefined;
    const tone = semanticConfig?.tone ?? toneProp;
    const variant = semanticConfig?.variant ?? variantProp;
    const resolvedRole = role ?? semanticConfig?.role;
    const resolvedAriaBusy = ariaBusy ?? semanticConfig?.busy;
    const shouldShowStatusLabel =
      showStatusLabel ?? semanticConfig?.statusLabel !== undefined;
    const statusLabel = semanticConfig?.statusLabel;

    return (
      <SurfacePanel
        aria-busy={resolvedAriaBusy === true ? true : undefined}
        className={cn(
          ALERT_PANEL_VARIANT_CLASS[variant],
          compact && ALERT_PANEL_COMPACT_CLASS[variant],
          ALERT_PANEL_TONE_CLASS[tone],
          ALERT_PANEL_BODY_TEXT_CLASS,
          className,
        )}
        data-af-feedback-variant={semantic}
        padding={padding}
        radius={radius}
        ref={ref}
        role={resolvedRole}
        {...props}
      >
        {shouldShowStatusLabel && statusLabel ? (
          <AlertPanelStatusLabel>{statusLabel}</AlertPanelStatusLabel>
        ) : null}
        {variant === "loading" && children === undefined ? (
          <>
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-4/5" />
          </>
        ) : (
          children
        )}
      </SurfacePanel>
    );
  },
);

export interface AlertPanelStatusLabelProps
  extends HTMLAttributes<HTMLSpanElement> {
  children?: ReactNode;
}

export const AlertPanelStatusLabel = forwardRef<
  HTMLSpanElement,
  AlertPanelStatusLabelProps
>(function AlertPanelStatusLabel({ className, ...props }, ref) {
  return (
    <span
      className={cn(ALERT_PANEL_STATUS_LABEL_CLASS, className)}
      ref={ref}
      {...props}
    />
  );
});

export interface AlertPanelTitleProps
  extends HTMLAttributes<HTMLHeadingElement> {
  children?: ReactNode;
}

export const AlertPanelTitle = forwardRef<
  HTMLHeadingElement,
  AlertPanelTitleProps
>(function AlertPanelTitle({ className, ...props }, ref) {
  return (
    <h3
      className={cn("m-0 text-body-medium !text-current", className)}
      ref={ref}
      {...props}
    />
  );
});

export interface AlertPanelTextProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
  children?: ReactNode;
  variant?: "body" | "supporting";
}

const ALERT_PANEL_SUPPORTING_TEXT_CLASS =
  "text-body-small text-on-surface-variant";

export const AlertPanelText = forwardRef<HTMLElement, AlertPanelTextProps>(
  function AlertPanelText(
    { as: Component = "p", className, variant = "body", ...props },
    ref,
  ) {
    return (
      <Component
        className={cn(
          "m-0 !text-current",
          variant === "body"
            ? ALERT_PANEL_BODY_TEXT_CLASS
            : ALERT_PANEL_SUPPORTING_TEXT_CLASS,
          className,
        )}
        ref={ref}
        {...props}
      />
    );
  },
);

export type { AlertPanelSemanticVariant } from "./alert-panel-semantics";
