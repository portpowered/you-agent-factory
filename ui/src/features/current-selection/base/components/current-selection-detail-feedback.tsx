import { forwardRef, type HTMLAttributes } from "react";

import { DashboardText } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";

type CurrentSelectionDetailFeedbackTone = "danger" | "neutral";

export interface CurrentSelectionDetailFeedbackProps
  extends HTMLAttributes<HTMLParagraphElement> {
  tone?: CurrentSelectionDetailFeedbackTone;
}

const CURRENT_SELECTION_DETAIL_FEEDBACK_TONE_CLASS: Record<
  CurrentSelectionDetailFeedbackTone,
  string
> = {
  danger: "text-on-error-container",
  neutral: "text-on-surface-variant",
};

export const CurrentSelectionDetailFeedback = forwardRef<
  HTMLParagraphElement,
  CurrentSelectionDetailFeedbackProps
>(function CurrentSelectionDetailFeedback(
  { className, tone = "neutral", ...props },
  ref,
) {
  return (
    <DashboardText
      className={cn(
        "m-0",
        CURRENT_SELECTION_DETAIL_FEEDBACK_TONE_CLASS[tone],
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});
