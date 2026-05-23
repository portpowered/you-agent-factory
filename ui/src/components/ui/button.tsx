import { forwardRef, type ButtonHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  tone?: "default" | "destructive" | "outline" | "secondary" | "ghost";
  size?: "default" | "icon" | "lg" | "sm";
}

const BUTTON_BASE_CLASS =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-xl border font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-focus-ring focus-visible:ring-offset-0 disabled:pointer-events-none disabled:border-af-border disabled:bg-af-surface-subtle disabled:text-af-text-disabled";
const BUTTON_TONE_CLASS: Record<NonNullable<ButtonProps["tone"]>, string> = {
  default:
    "border-af-accent bg-af-accent text-af-on-accent hover:brightness-105",
  destructive:
    "border-af-danger bg-af-danger text-af-on-danger hover:brightness-110",
  ghost: "border-transparent bg-transparent text-af-text-muted hover:bg-af-overlay hover:text-af-text",
  outline:
    "border-af-border bg-af-surface-raised text-af-text hover:border-af-border-strong hover:bg-af-overlay",
  secondary:
    "border-af-border-strong bg-af-surface-subtle text-af-accent hover:border-af-accent hover:bg-af-overlay",
};
const BUTTON_SIZE_CLASS: Record<NonNullable<ButtonProps["size"]>, string> = {
  default: "px-4 py-2.5 text-sm",
  icon: "h-11 w-11 px-0 py-0",
  lg: "px-5 py-3 text-base",
  sm: "min-h-9 rounded-lg px-3 py-2 text-xs",
};

export const buttonVariants = ({
  className,
  size = "default",
  tone = "default",
}: Pick<ButtonProps, "className" | "size" | "tone">) =>
  cn(BUTTON_BASE_CLASS, BUTTON_TONE_CLASS[tone], BUTTON_SIZE_CLASS[size], className);

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { className, size = "default", tone = "default", type = "button", ...props },
  ref,
) {
  return (
    <button
      className={buttonVariants({ className, size, tone })}
      ref={ref}
      type={type}
      {...props}
    />
  );
});
