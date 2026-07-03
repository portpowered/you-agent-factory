import { forwardRef, type HTMLAttributes, type ReactNode } from "react";

import { cn } from "../utilities/cn";

export interface GraphViewportSurfaceProps extends HTMLAttributes<HTMLElement> {
  children: ReactNode;
}

export const GraphViewportSurface = forwardRef<
  HTMLElement,
  GraphViewportSurfaceProps
>(function GraphViewportSurface({ children, className, role, ...props }, ref) {
  return (
    <section
      className={cn(
        "relative h-full max-h-full overflow-hidden rounded-3xl border shadow-none transition-colors",
        className,
      )}
      data-graph-viewport-surface="true"
      ref={ref}
      role={role ?? "region"}
      {...props}
    >
      {children}
    </section>
  );
});
