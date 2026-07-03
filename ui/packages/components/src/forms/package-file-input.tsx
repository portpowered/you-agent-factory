import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../utilities/cn";

export type PackageFileInputProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "type"
>;

const FILE_INPUT_CLASS =
  "block w-full rounded-xl px-3 py-3 text-sm text-on-surface-variant outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring file:mr-3 file:rounded-lg file:border-0 file:bg-surface-container-high file:px-3 file:py-2 file:text-sm file:font-semibold file:text-on-surface hover:bg-af-overlay aria-invalid:border-af-danger-border aria-invalid:ring-2 aria-invalid:ring-af-danger-border disabled:cursor-not-allowed disabled:bg-surface-container-low disabled:text-on-surface-disabled";

export const PackageFileInput = forwardRef<
  HTMLInputElement,
  PackageFileInputProps
>(function PackageFileInput({ className, ...props }, ref) {
  return (
    <input
      className={cn(FILE_INPUT_CLASS, className)}
      ref={ref}
      type="file"
      {...props}
    />
  );
});
