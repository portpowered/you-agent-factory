import { forwardRef, type ReactNode } from "react";

import { cn } from "../../lib/cn";
import { Button, type ButtonProps } from "./button";

const DASHBOARD_ACTION_BUTTON_BASE_CLASS =
  "relative shrink-0 rounded-lg border text-sm font-semibold";
const DASHBOARD_ACTION_BUTTON_SIZE_CLASS = {
  icon: "h-10 w-10 px-0 py-0",
  text: "min-h-10 px-3.5 py-2",
};
const DASHBOARD_ACTION_BUTTON_CONTENT_CLASS =
  "inline-flex items-center justify-center gap-2";
const DASHBOARD_ACTION_BUTTON_EXECUTING_CONTENT_CLASS = "opacity-0";
const DASHBOARD_ACTION_BUTTON_EXECUTING_OVERLAY_CLASS =
  "pointer-events-none absolute inset-0 inline-flex items-center justify-center";
const DASHBOARD_ACTION_BUTTON_SPINNER_CLASS = "size-4 animate-spin";

export interface DashboardActionButtonProps
  extends Omit<ButtonProps, "children" | "size"> {
  children: ReactNode;
  executing?: boolean;
  iconOnly?: boolean;
}

export const DashboardActionButton = forwardRef<
  HTMLButtonElement,
  DashboardActionButtonProps
>(function DashboardActionButton(
  {
    children,
    className,
    disabled = false,
    executing = false,
    iconOnly = false,
    tone = "outline",
    ...props
  },
  ref,
) {
  return (
    <Button
      aria-busy={executing || undefined}
      className={cn(
        DASHBOARD_ACTION_BUTTON_BASE_CLASS,
        iconOnly
          ? DASHBOARD_ACTION_BUTTON_SIZE_CLASS.icon
          : DASHBOARD_ACTION_BUTTON_SIZE_CLASS.text,
        className,
      )}
      disabled={disabled || executing}
      ref={ref}
      size={iconOnly ? "icon" : "sm"}
      tone={tone}
      {...props}
    >
      <span
        className={cn(
          DASHBOARD_ACTION_BUTTON_CONTENT_CLASS,
          executing && DASHBOARD_ACTION_BUTTON_EXECUTING_CONTENT_CLASS,
        )}
      >
        {children}
      </span>
      {executing ? (
        <span
          aria-hidden="true"
          className={DASHBOARD_ACTION_BUTTON_EXECUTING_OVERLAY_CLASS}
        >
          <DashboardActionButtonSpinner />
        </span>
      ) : null}
    </Button>
  );
});

function DashboardActionButtonSpinner() {
  return (
    <svg
      aria-hidden="true"
      className={DASHBOARD_ACTION_BUTTON_SPINNER_CLASS}
      fill="none"
      focusable="false"
      viewBox="0 0 16 16"
    >
      <circle
        className="text-af-text-disabled"
        cx="8"
        cy="8"
        r="6"
        stroke="currentColor"
        strokeWidth="1.5"
      />
      <path
        d="M8 2a6 6 0 0 1 6 6"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}
