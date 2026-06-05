import { forwardRef, type HTMLAttributes } from "react";
import { surfacePanelVariants } from "../../../../components/ui";
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
      className={surfacePanelVariants({
        className: cn("grid min-w-0 gap-2.5", className),
        radius: "lg",
        surface: "low",
        tone: highlighted ? "accent" : "default",
      })}
      ref={ref}
      {...props}
    />
  );
});
