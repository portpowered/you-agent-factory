import { forwardRef } from "react";

import { cn } from "../utilities/cn";
import { Button, type ButtonProps } from "./button";

const ICON_BUTTON_SHELL_BASE_CLASS = "relative shrink-0";
const ICON_BUTTON_SHELL_SIZE_CLASS = "h-10 w-10 rounded-lg";
const ICON_BUTTON_SHELL_TONE_CLASS = {
  dangerGhost:
    "text-on-surface-subtle hover:border-af-danger-border hover:bg-error-container hover:text-on-error-container",
} as const;

type IconButtonShellTone =
  | ButtonProps["tone"]
  | keyof typeof ICON_BUTTON_SHELL_TONE_CLASS;

export interface IconButtonShellProps
  extends Omit<ButtonProps, "size" | "tone"> {
  tone?: IconButtonShellTone;
}

export const IconButtonShell = forwardRef<
  HTMLButtonElement,
  IconButtonShellProps
>(function IconButtonShell({ className, tone = "outline", ...props }, ref) {
  const buttonTone = tone === "dangerGhost" ? "ghost" : tone;
  const toneClassName =
    tone === "dangerGhost"
      ? ICON_BUTTON_SHELL_TONE_CLASS.dangerGhost
      : undefined;

  return (
    <Button
      className={cn(
        ICON_BUTTON_SHELL_BASE_CLASS,
        ICON_BUTTON_SHELL_SIZE_CLASS,
        toneClassName,
        className,
      )}
      ref={ref}
      size="icon"
      tone={buttonTone}
      {...props}
    />
  );
});
