import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

export const INPUT_CLASS =
  "flex min-h-11 w-full rounded-xl border border-af-border bg-af-surface-raised px-3 py-2.5 text-sm text-af-text outline-none transition-colors placeholder:text-af-text-disabled focus-visible:border-af-border-strong focus-visible:ring-2 focus-visible:ring-af-focus-ring disabled:cursor-not-allowed disabled:border-af-border disabled:bg-af-surface-subtle disabled:text-af-text-disabled";

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, type = "text", ...props },
  ref,
) {
  return <input className={cn(INPUT_CLASS, className)} ref={ref} type={type} {...props} />;
});
