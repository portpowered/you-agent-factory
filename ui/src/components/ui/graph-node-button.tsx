import { forwardRef, type ButtonHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export type GraphNodeButtonProps = ButtonHTMLAttributes<HTMLButtonElement>;

const GRAPH_NODE_BUTTON_BASE_CLASS =
  "nodrag nopan cursor-pointer border-0 bg-transparent p-0 text-left text-inherit focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";

export const GraphNodeButton = forwardRef<
  HTMLButtonElement,
  GraphNodeButtonProps
>(function GraphNodeButton({ className, type = "button", ...props }, ref) {
  return (
    <button
      className={cn(GRAPH_NODE_BUTTON_BASE_CLASS, className)}
      ref={ref}
      type={type}
      {...props}
    />
  );
});
