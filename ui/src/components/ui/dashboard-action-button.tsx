import {
  Button,
  type ButtonProps,
  IconButtonShell,
} from "@you-agent-factory/components/primitives";
import { forwardRef, type ReactNode } from "react";
import { cn } from "../../lib/cn";

const DASHBOARD_ACTION_BUTTON_BASE_CLASS =
  "relative shrink-0 rounded-lg border text-sm font-semibold";
const DASHBOARD_ACTION_BUTTON_SIZE_CLASS = {
  text: "min-h-10 px-3.5 py-2",
};

export interface DashboardActionButtonProps
  extends Omit<ButtonProps, "children" | "size" | "loading"> {
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
  if (iconOnly) {
    return (
      <IconButtonShell
        className={cn(DASHBOARD_ACTION_BUTTON_BASE_CLASS, className)}
        disabled={disabled}
        loading={executing}
        ref={ref}
        tone={tone}
        {...props}
      >
        {children}
      </IconButtonShell>
    );
  }

  return (
    <Button
      className={cn(
        DASHBOARD_ACTION_BUTTON_BASE_CLASS,
        DASHBOARD_ACTION_BUTTON_SIZE_CLASS.text,
        className,
      )}
      disabled={disabled}
      loading={executing}
      ref={ref}
      size="sm"
      tone={tone}
      {...props}
    >
      {children}
    </Button>
  );
});
