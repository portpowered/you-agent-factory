import type { HTMLAttributes } from "react";

import { cn } from "../utilities/cn";

export function Skeleton({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      aria-hidden="true"
      className={cn("animate-pulse rounded-xl bg-af-overlay", className)}
      {...props}
    />
  );
}
