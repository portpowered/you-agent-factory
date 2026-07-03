import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../utilities/cn";

export type PackageInputProps = InputHTMLAttributes<HTMLInputElement>;

const INPUT_CLASS =
  "flex min-h-11 w-full rounded-xl border border-outline bg-surface-container-high px-3 py-2.5 text-sm text-on-surface outline-none transition-colors placeholder:text-on-surface-disabled focus-visible:border-outline-variant focus-visible:ring-2 focus-visible:ring-af-focus-ring aria-invalid:border-af-danger-border aria-invalid:ring-2 aria-invalid:ring-af-danger-border disabled:cursor-not-allowed disabled:border-outline disabled:bg-surface-container-low disabled:text-on-surface-disabled";

export function inputVariants({ className }: { className?: string } = {}) {
  return cn(INPUT_CLASS, className);
}

export const PackageInput = forwardRef<HTMLInputElement, PackageInputProps>(
  function PackageInput({ className, type = "text", ...props }, ref) {
    return (
      <input
        className={inputVariants({ className })}
        ref={ref}
        type={type}
        {...props}
      />
    );
  },
);
