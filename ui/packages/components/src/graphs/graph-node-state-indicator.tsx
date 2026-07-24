import { cn } from "../utilities/cn";
import {
  defaultGraphNodeStateLabel,
  GRAPH_NODE_STATE_INDICATOR_HEIGHT_CLASS,
  type GraphNodeState,
} from "./graph-node-state";

export interface GraphNodeStateIndicatorProps {
  state: GraphNodeState;
  stateLabel?: string;
}

export function GraphNodeStateIndicator({
  state,
  stateLabel,
}: GraphNodeStateIndicatorProps) {
  const label = stateLabel ?? defaultGraphNodeStateLabel(state);
  const showIndicator = state === "loading" || state === "error";

  return (
    <div
      aria-hidden={showIndicator ? undefined : true}
      className={cn(
        GRAPH_NODE_STATE_INDICATOR_HEIGHT_CLASS,
        "flex items-center gap-2 text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
        state === "error"
          ? "text-on-error-container"
          : "text-on-surface-variant",
        !showIndicator && "invisible",
      )}
      data-graph-node-state-indicator={showIndicator ? state : undefined}
    >
      {state === "loading" ? (
        <>
          <span
            aria-hidden="true"
            className="inline-block h-4 w-4 shrink-0 animate-spin rounded-full border-2 border-outline border-t-primary"
            data-graph-node-loading-spinner="true"
          />
          <span>{label ?? "Loading"}</span>
        </>
      ) : state === "error" ? (
        <span role="alert">{label ?? "Error"}</span>
      ) : (
        <span aria-hidden="true">&nbsp;</span>
      )}
    </div>
  );
}
