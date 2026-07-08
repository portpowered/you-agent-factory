import { forwardRef, type HTMLAttributes } from "react";

import { BODY_TEXT_CLASS } from "../primitives/typography-roles";
import { cn } from "../utilities/cn";

export interface DescriptionListProps
  extends HTMLAttributes<HTMLDListElement> {}

export const DescriptionList = forwardRef<
  HTMLDListElement,
  DescriptionListProps
>(function DescriptionList({ className, ...props }, ref) {
  return (
    <dl
      className={cn(
        "m-0 grid min-w-0 gap-1.5 [&_dd]:m-0 [&_dd]:min-w-0 [&_div]:grid [&_div]:min-w-0 [&_div]:gap-2",
        BODY_TEXT_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});
