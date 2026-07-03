import {
  type ElementType,
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { SurfacePanel, type SurfacePanelProps } from "../layout/surface-panel";
import { cn } from "../utilities/cn";

export type AlertPanelTone =
  | "danger"
  | "info"
  | "neutral"
  | "success"
  | "warning";
export type AlertPanelVariant = "default" | "empty";

export interface AlertPanelProps
  extends Omit<SurfacePanelProps, "surface" | "tone"> {
  compact?: boolean;
  tone?: AlertPanelTone;
  variant?: AlertPanelVariant;
}

const ALERT_PANEL_TONE_CLASS: Record<AlertPanelTone, string> = {
  danger: "border-af-danger-border bg-error-container text-on-error-container",
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
};
const ALERT_PANEL_COMPACT_CLASS: Record<AlertPanelVariant, string> = {
  default: "",
  empty: "min-h-0",
};

const ALERT_PANEL_BODY_TEXT_CLASS = "text-body-medium text-on-surface-variant";

export const AlertPanel = forwardRef<HTMLDivElement, AlertPanelProps>(
  function AlertPanel(
    {
      className,
      compact = false,
      padding = "default",
      radius = "xl",
      tone = "warning",
      variant = "default",
      ...props
    },
    ref,
  ) {
    return (
      <SurfacePanel
        className={cn(
          ALERT_PANEL_VARIANT_CLASS[variant],
          compact && ALERT_PANEL_COMPACT_CLASS[variant],
          ALERT_PANEL_TONE_CLASS[tone],
          ALERT_PANEL_BODY_TEXT_CLASS,
          className,
        )}
        padding={padding}
        radius={radius}
        ref={ref}
        {...props}
      />
    );
  },
);

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
