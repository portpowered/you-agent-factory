import {
  type GraphViewportSurfaceProps,
  GraphViewportSurface as PackageGraphViewportSurface,
} from "@you-agent-factory/components/graphs";
import { forwardRef } from "react";

export const GraphViewportSurface = forwardRef<
  HTMLElement,
  GraphViewportSurfaceProps
>(function GraphViewportSurface(props, ref) {
  return (
    <PackageGraphViewportSurface
      data-dashboard-graph-frame="true"
      ref={ref}
      {...props}
    />
  );
});
