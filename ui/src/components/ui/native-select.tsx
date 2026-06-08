import { ChevronDown } from "lucide-react";
import { forwardRef, type SelectHTMLAttributes } from "react";

import { inputVariants } from "./input";

export type NativeSelectProps = SelectHTMLAttributes<HTMLSelectElement>;

export const NativeSelect = forwardRef<HTMLSelectElement, NativeSelectProps>(
  function NativeSelect({ children, className, ...props }, ref) {
    return (
      <div className="relative">
        <select
          className={inputVariants({
            className: `appearance-none pr-10 ${className ?? ""}`,
          })}
          ref={ref}
          {...props}
        >
          {children}
        </select>
        <ChevronDown
          aria-hidden="true"
          className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-af-text-subtle"
          focusable="false"
          strokeWidth={1.8}
        />
      </div>
    );
  },
);
