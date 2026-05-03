import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

export const INPUT_CLASS =
  "flex min-h-11 w-full rounded-xl border border-af-overlay/14 bg-af-canvas/82 px-3 py-[0.65rem] text-sm text-af-ink outline-none transition-colors placeholder:text-af-ink/42 focus-visible:border-af-accent focus-visible:ring-2 focus-visible:ring-af-accent/25 disabled:cursor-not-allowed disabled:border-af-overlay/10 disabled:bg-af-overlay/6 disabled:text-af-ink/48";

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, type = "text", ...props },
  ref,
) {
  return <input className={cn(INPUT_CLASS, className)} ref={ref} type={type} {...props} />;
});

