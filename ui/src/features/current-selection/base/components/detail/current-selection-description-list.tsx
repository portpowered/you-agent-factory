import { forwardRef, type HTMLAttributes } from "react";

import { DescriptionList } from "@you-agent-factory/components/data-display";
import { cn } from "../../../../../lib/cn";

export interface CurrentSelectionDescriptionListProps
  extends HTMLAttributes<HTMLDListElement> {}

export const CurrentSelectionDescriptionList = forwardRef<
  HTMLDListElement,
  CurrentSelectionDescriptionListProps
>(function CurrentSelectionDescriptionList({ className, ...props }, ref) {
  return (
    <DescriptionList
      className={cn("[&_div]:grid-cols-[8.5rem_minmax(0,1fr)]", className)}
      ref={ref}
      {...props}
    />
  );
});
