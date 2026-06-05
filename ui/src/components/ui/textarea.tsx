import { forwardRef, type TextareaHTMLAttributes } from "react";

import { cn } from "../../lib/cn";
import { inputVariants } from "./input";

const TEXTAREA_PLAIN_CLASS =
  "m-0 w-full resize-none border-0 bg-transparent p-0 text-sm leading-6 text-on-surface outline-none";

export interface TextareaProps
  extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  variant?: "field" | "plain";
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  function Textarea({ className, variant = "field", ...props }, ref) {
    return (
      <textarea
        className={
          variant === "plain"
            ? cn(TEXTAREA_PLAIN_CLASS, className)
            : inputVariants({
                className: `min-h-28 resize-y py-3 ${className ?? ""}`,
              })
        }
        ref={ref}
        {...props}
      />
    );
  },
);
