import { forwardRef, type TextareaHTMLAttributes } from "react";

import { cn } from "../../lib/cn";
import { inputVariants } from "./input";
import { STYLED_SCROLLBAR_CLASS } from "./styled-scrollbar";

const TEXTAREA_PLAIN_CLASS =
  "m-0 w-full resize-none border-0 bg-transparent p-0 text-sm leading-6 text-on-surface outline-none";

const TEXTAREA_FIELD_CLASS = cn(
  "min-h-28 max-h-52 resize-none overflow-y-auto py-3",
  STYLED_SCROLLBAR_CLASS,
);

export interface TextareaProps
  extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  variant?: "field" | "plain";
}

export function textareaVariants({ className }: { className?: string } = {}) {
  return inputVariants({ className: cn(TEXTAREA_FIELD_CLASS, className) });
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  function Textarea({ className, variant = "field", ...props }, ref) {
    return (
      <textarea
        className={
          variant === "plain"
            ? cn(TEXTAREA_PLAIN_CLASS, className)
            : textareaVariants({ className })
        }
        ref={ref}
        {...props}
      />
    );
  },
);
