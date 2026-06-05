import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export interface CheckboxProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {}

const CHECKBOX_CLASS =
  "size-4 rounded border border-outline bg-surface-container-high text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring disabled:cursor-not-allowed disabled:border-outline disabled:bg-surface-container-low";

export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(
  function Checkbox({ className, ...props }, ref) {
    return (
      <input
        className={cn(CHECKBOX_CLASS, className)}
        ref={ref}
        type="checkbox"
        {...props}
      />
    );
  },
);
