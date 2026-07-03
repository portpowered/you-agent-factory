import { forwardRef, type SelectHTMLAttributes } from "react";

import { inputVariants } from "./package-input";
import { SelectChevronDownIcon } from "./select-icons";

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
        <SelectChevronDownIcon className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-af-text-subtle" />
      </div>
    );
  },
);
