import { forwardRef, type ReactNode } from "react";

import { cn } from "../../lib/cn";
import {
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "./dashboard-typography";
import {
  DisclosureButton,
  type DisclosureButtonProps,
} from "./disclosure-button";
import { ExpandablePanelIcon } from "./expandable-panel-icon";

export type ExpandablePanelTriggerVariant = "section" | "compact" | "outline";

const EXPANDABLE_PANEL_TRIGGER_VARIANT_CLASS: Record<
  ExpandablePanelTriggerVariant,
  string
> = {
  section: cn(
    "shrink-0 cursor-pointer rounded-lg border border-af-border bg-af-surface-raised px-2.5 py-2 text-af-text-muted transition hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-accent disabled:cursor-not-allowed disabled:border-af-border disabled:bg-af-surface-subtle disabled:text-af-text-disabled",
    DASHBOARD_SUPPORTING_TEXT_CLASS,
  ),
  compact: cn(
    "min-h-9 shrink-0 gap-1.5 border-af-border bg-af-surface-raised px-2.5 py-1.5 text-xs text-af-text-muted hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text",
    DASHBOARD_SUPPORTING_TEXT_CLASS,
  ),
  outline: cn("shrink-0 gap-1.5", DASHBOARD_SUPPORTING_LABEL_CLASS),
};

type ExpandablePanelTriggerBaseProps = Omit<
  DisclosureButtonProps,
  "expanded" | "controlsID" | "children" | "onClick"
> & {
  controlsID: string;
  expanded: boolean;
  variant?: ExpandablePanelTriggerVariant;
  onClick?: DisclosureButtonProps["onClick"];
  onToggle?: (expanded: boolean) => void;
};

type ExpandablePanelTriggerWithLabelProps = ExpandablePanelTriggerBaseProps & {
  children: ReactNode;
  "aria-label"?: string;
};

type ExpandablePanelTriggerIconOnlyProps = ExpandablePanelTriggerBaseProps & {
  children?: never;
  "aria-label": string;
};

export type ExpandablePanelTriggerProps =
  | ExpandablePanelTriggerWithLabelProps
  | ExpandablePanelTriggerIconOnlyProps;

export const ExpandablePanelTrigger = forwardRef<
  HTMLButtonElement,
  ExpandablePanelTriggerProps
>(function ExpandablePanelTrigger(
  {
    children,
    className,
    controlsID,
    expanded,
    onClick,
    onToggle,
    type = "button",
    variant = "section",
    ...props
  },
  ref,
) {
  const handleClick: DisclosureButtonProps["onClick"] = (event) => {
    onClick?.(event);
    if (!event.defaultPrevented) {
      onToggle?.(!expanded);
    }
  };

  return (
    <DisclosureButton
      className={cn(
        "inline-flex items-center justify-center",
        EXPANDABLE_PANEL_TRIGGER_VARIANT_CLASS[variant],
        className,
      )}
      controlsID={controlsID}
      expanded={expanded}
      onClick={handleClick}
      ref={ref}
      type={type}
      {...props}
    >
      <ExpandablePanelIcon expanded={expanded} />
      {children ? <span className="min-w-0">{children}</span> : null}
    </DisclosureButton>
  );
});
