import { forwardRef, type TextareaHTMLAttributes } from "react";

import { cn } from "../utilities/cn";
import { inputVariants } from "./package-input";

const TEXTAREA_PLAIN_CLASS =
  "m-0 w-full resize-none border-0 bg-transparent p-0 text-sm leading-6 text-on-surface outline-none";

const TEXTAREA_FIELD_CLASS =
  "min-h-28 max-h-52 resize-none overflow-y-auto py-3";

export type PackageTextareaProps =
  TextareaHTMLAttributes<HTMLTextAreaElement> & {
    variant?: "field" | "plain";
  };

export function textareaVariants({ className }: { className?: string } = {}) {
  return inputVariants({ className: cn(TEXTAREA_FIELD_CLASS, className) });
}

export const PackageTextarea = forwardRef<
  HTMLTextAreaElement,
  PackageTextareaProps
>(function PackageTextarea({ className, variant = "field", ...props }, ref) {
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
});
