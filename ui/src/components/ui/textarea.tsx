import { forwardRef, type TextareaHTMLAttributes } from "react";

import { cn } from "../../lib/cn";
import { INPUT_CLASS } from "./input";

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { className, ...props },
  ref,
) {
  return (
    <textarea
      className={cn(INPUT_CLASS, "min-h-28 resize-y py-3", className)}
      ref={ref}
      {...props}
    />
  );
});
