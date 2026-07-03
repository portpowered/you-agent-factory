import { type ButtonHTMLAttributes, forwardRef } from "react";

import { cn } from "../utilities/cn";
import {
  type GraphNodeState,
  graphNodeButtonIsDisabled,
  graphNodeButtonStateAttributes,
  graphNodeButtonStateClassName,
} from "./graph-node-state";

export type GraphNodeButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  graphState?: GraphNodeState;
  stateLabel?: string;
};

export const GRAPH_NODE_BUTTON_BASE_CLASS =
  "nodrag nopan cursor-pointer border-0 bg-transparent p-0 text-left text-inherit focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring disabled:cursor-not-allowed disabled:opacity-100";

export const GraphNodeButton = forwardRef<
  HTMLButtonElement,
  GraphNodeButtonProps
>(function GraphNodeButton(
  {
    className,
    disabled,
    graphState = "default",
    onClick,
    stateLabel,
    type = "button",
    ...props
  },
  ref,
) {
  const isDisabled = graphNodeButtonIsDisabled(graphState, disabled);

  return (
    <button
      className={cn(
        GRAPH_NODE_BUTTON_BASE_CLASS,
        graphNodeButtonStateClassName(graphState),
        className,
      )}
      disabled={isDisabled}
      onClick={isDisabled ? undefined : onClick}
      ref={ref}
      type={type}
      {...graphNodeButtonStateAttributes(graphState, stateLabel)}
      {...props}
    />
  );
});
