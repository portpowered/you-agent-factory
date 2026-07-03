import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../utilities/cn";

export type PackageFileInputProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "type"
>;

const FILE_INPUT_CLASS =
  "block w-full px-3 py-3 text-sm text-on-surface-variant file:mr-3 file:rounded-lg file:border-0 file:bg-surface-container-high file:px-3 file:py-2 file:text-sm file:font-semibold file:text-on-surface hover:bg-af-overlay";

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
