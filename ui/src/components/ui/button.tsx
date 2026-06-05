import { Slot } from "@radix-ui/react-slot";
import { type ButtonHTMLAttributes, forwardRef } from "react";

import { cn } from "../../lib/cn";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean;
  tone?:
    | "default"
    | "destructive"
    | "outline"
    | "secondary"
    | "ghost"
    | "warning";
  size?: "default" | "icon" | "iconPill" | "lg" | "pill" | "sm";
}

const BUTTON_BASE_CLASS =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-xl border font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-focus-ring focus-visible:ring-offset-0 disabled:pointer-events-none disabled:border-outline disabled:bg-surface-container-low disabled:text-on-surface-disabled";
const BUTTON_TONE_CLASS: Record<NonNullable<ButtonProps["tone"]>, string> = {
  default:
    "border-primary bg-primary text-on-primary hover:border-on-primary-container hover:bg-on-primary-container",
  destructive:
    "border-error bg-error text-on-error hover:border-af-danger-hover hover:bg-af-danger-hover",
  ghost:
    "border-transparent bg-transparent text-on-surface-variant hover:bg-af-overlay hover:text-on-surface",
  outline:
    "border-outline bg-surface-container-high text-on-surface hover:border-outline-variant hover:bg-af-overlay",
  secondary:
    "border-outline-variant bg-surface-container-low text-primary hover:border-primary hover:bg-af-overlay",
  warning:
    "border-af-warning-border bg-warning-container text-on-warning-container hover:border-af-warning-border hover:bg-warning-container hover:text-on-warning-container",
};
const BUTTON_SIZE_CLASS: Record<NonNullable<ButtonProps["size"]>, string> = {
  default: "px-4 py-2.5 text-sm",
  icon: "h-11 w-11 px-0 py-0",
  iconPill: "h-10 min-h-10 w-10 rounded-full px-0 py-0",
  lg: "px-5 py-3 text-base",
  pill: "min-h-9 rounded-full px-3 py-2 text-xs",
  sm: "min-h-9 rounded-lg px-3 py-2 text-xs",
};

export const buttonVariants = ({
  className,
  size = "default",
  tone = "default",
}: Pick<ButtonProps, "className" | "size" | "tone">) =>
  cn(
    BUTTON_BASE_CLASS,
    BUTTON_TONE_CLASS[tone],
    BUTTON_SIZE_CLASS[size],
    className,
  );

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(
    {
      asChild = false,
      className,
      size = "default",
      tone = "default",
      type = "button",
      ...props
    },
    ref,
  ) {
    const Component = asChild ? Slot : "button";
    return (
      <Component
        className={buttonVariants({ className, size, tone })}
        ref={ref}
        {...(!asChild ? { type } : undefined)}
        {...props}
      />
    );
  },
);
