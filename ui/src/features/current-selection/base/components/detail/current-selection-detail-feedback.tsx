import { forwardRef, type HTMLAttributes } from "react";

import { FormDescription, FormError } from "@you-agent-factory/components/forms";
import { cn } from "../../../../../lib/cn";

type CurrentSelectionDetailFeedbackTone = "danger" | "neutral";
type CurrentSelectionDetailFeedbackRole = "alert" | "status";

export interface CurrentSelectionDetailFeedbackProps
  extends HTMLAttributes<HTMLParagraphElement> {
  tone?: CurrentSelectionDetailFeedbackTone;
}

export const CurrentSelectionDetailFeedback = forwardRef<
  HTMLParagraphElement,
  CurrentSelectionDetailFeedbackProps
>(function CurrentSelectionDetailFeedback(
  { className, role, tone = "neutral", ...props },
  ref,
) {
  if (tone === "danger") {
    return (
      <FormError
        className={className}
        ref={ref}
        role={getFeedbackErrorRole(role)}
        {...props}
      />
    );
  }

  return (
    <FormDescription
      className={cn("text-on-surface-variant", className)}
      ref={ref}
      variant="body"
      {...props}
    />
  );
});

function getFeedbackErrorRole(
  role: HTMLAttributes<HTMLParagraphElement>["role"],
): CurrentSelectionDetailFeedbackRole {
  if (role === "alert") {
    return "alert";
  }

  if (role === "status") {
    return "status";
  }

  return "status";
}
