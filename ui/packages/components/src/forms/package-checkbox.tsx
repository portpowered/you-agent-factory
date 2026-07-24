import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../utilities/cn";

export type PackageCheckboxProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "type"
>;

const CHECKBOX_ROOT_CLASS =
  "relative inline-flex size-4 shrink-0 items-center justify-center [&:has(:disabled)]:cursor-not-allowed";

function CheckboxCheckIcon() {
  return (
    <svg
      aria-hidden="true"
      className="size-3 opacity-0"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2.5}
      viewBox="0 0 24 24"
    >
      <path d="M20 6 9 17l-5-5" />
    </svg>
  );
}

export const PackageCheckbox = forwardRef<
  HTMLInputElement,
  PackageCheckboxProps
>(function PackageCheckbox({ className, ...props }, ref) {
  return (
    <span className={cn(CHECKBOX_ROOT_CLASS, className)}>
      <input className="peer sr-only" ref={ref} type="checkbox" {...props} />
      <span
        aria-hidden="true"
        className="pointer-events-none flex size-4 items-center justify-center rounded border border-outline bg-surface-container-high text-on-primary transition-colors peer-focus-visible:outline-none peer-focus-visible:ring-2 peer-focus-visible:ring-af-focus-ring peer-disabled:border-outline peer-disabled:bg-surface-container-low peer-checked:border-primary peer-checked:bg-primary peer-aria-invalid:ring-2 peer-aria-invalid:ring-af-danger-border peer-checked:[&_svg]:opacity-100"
      >
        <CheckboxCheckIcon />
      </span>
    </span>
  );
});
