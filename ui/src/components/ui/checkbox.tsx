import { Check } from "lucide-react";
import { forwardRef, type InputHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export interface CheckboxProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {}

const CHECKBOX_ROOT_CLASS =
  "relative inline-flex size-4 shrink-0 items-center justify-center [&:has(:disabled)]:cursor-not-allowed";

const CHECKBOX_INPUT_CLASS = "peer sr-only";

const CHECKBOX_INDICATOR_CLASS =
  "pointer-events-none flex size-4 items-center justify-center rounded border border-outline bg-surface-container-high text-on-primary transition-colors peer-focus-visible:outline-none peer-focus-visible:ring-2 peer-focus-visible:ring-af-focus-ring peer-disabled:border-outline peer-disabled:bg-surface-container-low peer-checked:border-primary peer-checked:bg-primary peer-aria-invalid:ring-2 peer-aria-invalid:ring-af-danger-border peer-checked:[&_svg]:opacity-100";

export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(
  function Checkbox({ className, ...props }, ref) {
    return (
      <span className={cn(CHECKBOX_ROOT_CLASS, className)}>
        <input
          className={CHECKBOX_INPUT_CLASS}
          ref={ref}
          type="checkbox"
          {...props}
        />
        <span aria-hidden="true" className={CHECKBOX_INDICATOR_CLASS}>
          <Check className="size-3 opacity-0" strokeWidth={2.5} />
        </span>
      </span>
    );
  },
);
