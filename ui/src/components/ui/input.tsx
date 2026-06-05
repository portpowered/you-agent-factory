import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

const INPUT_CLASS =
  "flex min-h-11 w-full rounded-xl border border-outline bg-surface-container-high px-3 py-2.5 text-sm text-on-surface outline-none transition-colors placeholder:text-on-surface-disabled focus-visible:border-outline-variant focus-visible:ring-2 focus-visible:ring-af-focus-ring disabled:cursor-not-allowed disabled:border-outline disabled:bg-surface-container-low disabled:text-on-surface-disabled";

export function inputVariants({ className }: { className?: string } = {}) {
  return cn(INPUT_CLASS, className);
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, type = "text", ...props },
  ref,
) {
  return (
    <input
      className={inputVariants({ className })}
      ref={ref}
      type={type}
      {...props}
    />
  );
});
