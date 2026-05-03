import { forwardRef, type ButtonHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  tone?: "default" | "destructive" | "outline" | "secondary" | "ghost";
  size?: "default" | "icon" | "lg" | "sm";
}

const BUTTON_BASE_CLASS =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-xl border font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-accent/25 focus-visible:ring-offset-0 disabled:pointer-events-none disabled:border-af-overlay/12 disabled:bg-af-overlay/6 disabled:text-af-ink/48";
const BUTTON_TONE_CLASS: Record<NonNullable<ButtonProps["tone"]>, string> = {
  default:
    "border-af-accent/45 bg-af-accent text-af-canvas hover:border-af-accent hover:bg-af-accent-glow",
  destructive:
    "border-af-danger/45 bg-af-danger text-af-ink hover:border-af-danger-bright hover:bg-af-danger-bright",
  ghost: "border-transparent bg-transparent text-af-ink/76 hover:bg-af-overlay/8 hover:text-af-ink",
  outline:
    "border-af-overlay/14 bg-af-surface/78 text-af-ink/82 hover:border-af-accent/35 hover:bg-af-overlay/8",
  secondary:
    "border-af-accent/24 bg-af-accent/10 text-af-accent hover:bg-af-accent/16 hover:text-af-accent-glow",
};
const BUTTON_SIZE_CLASS: Record<NonNullable<ButtonProps["size"]>, string> = {
  default: "px-4 py-[0.65rem] text-sm",
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
  return <button className={buttonVariants({ className, size, tone })} ref={ref} type={type} {...props} />;
});

