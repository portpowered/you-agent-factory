import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../../../../../lib/cn";

export interface CurrentSelectionFormFieldsProps
  extends HTMLAttributes<HTMLDivElement> {}

export const CurrentSelectionFormFields = forwardRef<
  HTMLDivElement,
  CurrentSelectionFormFieldsProps
>(function CurrentSelectionFormFields({ className, ...props }, ref) {
  return (
    <div
      className={cn("grid grid-cols-1 gap-3", className)}
      ref={ref}
      {...props}
    />
  );
});

export interface CurrentSelectionFormFieldProps
  extends HTMLAttributes<HTMLDivElement> {}

export const CurrentSelectionFormField = forwardRef<
  HTMLDivElement,
  CurrentSelectionFormFieldProps
>(function CurrentSelectionFormField({ className, ...props }, ref) {
  return <div className={cn("grid gap-2", className)} ref={ref} {...props} />;
});
