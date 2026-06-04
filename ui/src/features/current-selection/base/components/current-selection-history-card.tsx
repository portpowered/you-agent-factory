import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../../../../lib/cn";

export interface CurrentSelectionHistoryCardProps
  extends HTMLAttributes<HTMLElement> {
  highlighted?: boolean;
}

export const CurrentSelectionHistoryCard = forwardRef<
  HTMLElement,
  CurrentSelectionHistoryCardProps
>(function CurrentSelectionHistoryCard(
  { className, highlighted = false, ...props },
  ref,
) {
  return (
    <article
      className={cn(
        "grid min-w-0 gap-2.5 rounded-lg border border-outline bg-surface-container-low p-3",
        highlighted && "border-primary text-on-surface",
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});
