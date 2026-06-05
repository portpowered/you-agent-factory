import { forwardRef, type HTMLAttributes } from "react";

import { DashboardCode, DashboardText } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";

type CurrentSelectionSupportingTextTone = "notice" | "status";

export interface CurrentSelectionSupportingTextProps
  extends HTMLAttributes<HTMLParagraphElement> {
  tone?: CurrentSelectionSupportingTextTone;
}

const SUPPORTING_TEXT_TONE_CLASS: Record<
  CurrentSelectionSupportingTextTone,
  string
> = {
  notice: "text-on-surface-variant",
  status: "text-on-surface-subtle",
};

export const CurrentSelectionSupportingText = forwardRef<
  HTMLParagraphElement,
  CurrentSelectionSupportingTextProps
>(function CurrentSelectionSupportingText(
  { className, tone = "notice", ...props },
  ref,
) {
  return (
    <DashboardText
      className={cn(
        "m-0",
        SUPPORTING_TEXT_TONE_CLASS[tone],
        className,
      )}
      ref={ref}
      variant="supporting"
      {...props}
    />
  );
});

export interface CurrentSelectionSubtleCodeProps
  extends HTMLAttributes<HTMLElement> {}

export const CurrentSelectionSubtleCode = forwardRef<
  HTMLElement,
  CurrentSelectionSubtleCodeProps
>(function CurrentSelectionSubtleCode({ className, ...props }, ref) {
  return (
    <DashboardCode
      className={cn("text-xs text-on-surface-variant", className)}
      ref={ref}
      {...props}
    />
  );
});
