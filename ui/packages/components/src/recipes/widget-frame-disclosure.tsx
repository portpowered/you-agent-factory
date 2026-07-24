import {
  type ButtonHTMLAttributes,
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../utilities/cn";

import { WIDGET_FRAME_BODY_TEXT_CLASS } from "./widget-frame-typography";

const WIDGET_FRAME_DISCLOSURE_CLASS = "grid min-w-0 gap-2.5";
const WIDGET_FRAME_DISCLOSURE_TRIGGER_CLASS = cn(
  "inline-flex min-h-9 shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-lg border border-outline bg-surface-container-high px-2.5 py-2 text-on-surface-variant transition hover:border-outline-variant hover:bg-af-overlay hover:text-on-surface focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent disabled:cursor-not-allowed disabled:border-outline disabled:bg-surface-container-low disabled:text-on-surface-disabled",
  WIDGET_FRAME_BODY_TEXT_CLASS,
);
const WIDGET_FRAME_DISCLOSURE_ICON_CLASS =
  "h-3.5 w-3.5 shrink-0 text-current transition-transform duration-150";

export interface WidgetFrameDisclosureProps
  extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
}

export function WidgetFrameDisclosure({
  children,
  className,
  ...props
}: WidgetFrameDisclosureProps) {
  return (
    <div className={cn(WIDGET_FRAME_DISCLOSURE_CLASS, className)} {...props}>
      {children}
    </div>
  );
}

export interface WidgetFrameDisclosureIconProps {
  expanded: boolean;
  className?: string;
}

export function WidgetFrameDisclosureIcon({
  className,
  expanded,
}: WidgetFrameDisclosureIconProps) {
  return (
    <svg
      aria-hidden="true"
      className={cn(
        WIDGET_FRAME_DISCLOSURE_ICON_CLASS,
        expanded ? "rotate-180" : "rotate-0",
        className,
      )}
      fill="none"
      focusable="false"
      viewBox="0 0 16 16"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M4 6l4 4 4-4"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

type WidgetFrameDisclosureTriggerBaseProps = Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "aria-expanded" | "aria-controls" | "children" | "type"
> & {
  controlsID: string;
  expanded: boolean;
  onExpandedChange?: (expanded: boolean) => void;
  type?: "button";
};

type WidgetFrameDisclosureTriggerWithLabelProps =
  WidgetFrameDisclosureTriggerBaseProps & {
    children: ReactNode;
    "aria-label"?: string;
  };

type WidgetFrameDisclosureTriggerIconOnlyProps =
  WidgetFrameDisclosureTriggerBaseProps & {
    children?: never;
    "aria-label": string;
  };

export type WidgetFrameDisclosureTriggerProps =
  | WidgetFrameDisclosureTriggerWithLabelProps
  | WidgetFrameDisclosureTriggerIconOnlyProps;

export const WidgetFrameDisclosureTrigger = forwardRef<
  HTMLButtonElement,
  WidgetFrameDisclosureTriggerProps
>(function WidgetFrameDisclosureTrigger(
  {
    children,
    className,
    controlsID,
    expanded,
    onClick,
    onExpandedChange,
    type = "button",
    ...props
  },
  ref,
) {
  const handleClick: ButtonHTMLAttributes<HTMLButtonElement>["onClick"] = (
    event,
  ) => {
    onClick?.(event);
    if (!event.defaultPrevented) {
      onExpandedChange?.(!expanded);
    }
  };

  return (
    <button
      aria-controls={controlsID}
      aria-expanded={expanded}
      className={cn(WIDGET_FRAME_DISCLOSURE_TRIGGER_CLASS, className)}
      onClick={handleClick}
      ref={ref}
      type={type}
      {...props}
    >
      <WidgetFrameDisclosureIcon expanded={expanded} />
      {children}
    </button>
  );
});

export interface WidgetFrameDisclosurePanelProps
  extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
  expanded: boolean;
  id: string;
}

export function WidgetFrameDisclosurePanel({
  children,
  className,
  expanded,
  id,
  ...props
}: WidgetFrameDisclosurePanelProps) {
  return (
    <div
      className={cn("min-w-0", className)}
      hidden={!expanded}
      id={id}
      {...props}
    >
      {children}
    </div>
  );
}
