import { cn } from "../utilities/cn";

export type GraphNodeState =
  | "default"
  | "selected"
  | "disabled"
  | "loading"
  | "error";

export const GRAPH_NODE_CONTENT_MIN_HEIGHT_CLASS = "min-h-12";

export const GRAPH_NODE_STATE_INDICATOR_HEIGHT_CLASS = "min-h-5";

export function graphNodeShellStateClassName(state: GraphNodeState): string {
  switch (state) {
    case "selected":
      return cn(
        "border-2 border-primary bg-primary-container",
        "shadow-[0_0_0_1px_rgb(from_var(--color-primary)_r_g_b_/_0.28),0_0_0_4px_rgb(from_var(--color-primary)_r_g_b_/_0.08)]",
      );
    case "error":
      return cn(
        "border-2 border-dashed border-error bg-error-container",
        "shadow-[0_0_0_3px_var(--color-error-container)]",
      );
    case "disabled":
      return "border-outline-variant bg-surface-container text-on-surface";
    case "loading":
      return "border-outline bg-surface";
    default:
      return "";
  }
}

export function graphNodeButtonStateClassName(state: GraphNodeState): string {
  switch (state) {
    case "selected":
      return "ring-2 ring-primary/30";
    case "error":
      return "ring-2 ring-error/40";
    case "disabled":
    case "loading":
      return "cursor-not-allowed";
    default:
      return "";
  }
}

export function graphNodeShellStateAttributes(
  state: GraphNodeState,
  stateLabel?: string,
): {
  "aria-busy"?: boolean;
  "aria-disabled"?: boolean;
  "aria-invalid"?: boolean;
  "aria-label"?: string;
  "aria-selected"?: boolean;
  "data-graph-node-state"?: GraphNodeState;
} {
  return {
    "aria-busy": state === "loading" ? true : undefined,
    "aria-disabled": state === "disabled" ? true : undefined,
    "aria-invalid": state === "error" ? true : undefined,
    "aria-label": stateLabel,
    "aria-selected": state === "selected" ? true : undefined,
    "data-graph-node-state": state === "default" ? undefined : state,
  };
}

export function graphNodeButtonStateAttributes(
  state: GraphNodeState,
  stateLabel?: string,
): {
  "aria-busy"?: boolean;
  "aria-disabled"?: boolean;
  "aria-invalid"?: boolean;
  "aria-label"?: string;
  "aria-pressed"?: boolean;
  "data-graph-node-state"?: GraphNodeState;
} {
  return {
    "aria-busy": state === "loading" ? true : undefined,
    "aria-disabled": state === "disabled" ? true : undefined,
    "aria-invalid": state === "error" ? true : undefined,
    "aria-label": stateLabel,
    "aria-pressed": state === "selected" ? true : undefined,
    "data-graph-node-state": state === "default" ? undefined : state,
  };
}

export function graphNodeButtonIsDisabled(
  state: GraphNodeState,
  disabled?: boolean,
): boolean {
  return disabled === true || state === "disabled" || state === "loading";
}

export function defaultGraphNodeStateLabel(
  state: GraphNodeState,
): string | undefined {
  switch (state) {
    case "selected":
      return "Selected node";
    case "disabled":
      return "Disabled node";
    case "loading":
      return "Loading node";
    case "error":
      return "Error node";
    default:
      return undefined;
  }
}
