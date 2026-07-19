import { safeParseFactoryVisualizationLayout } from "@you-agent-factory/client";
import type {
  FactoryActivityProjection,
  FactoryLoadProjection,
  FactoryTopologyNode,
  FactoryTopologyProjection,
} from "@you-agent-factory/factory-replay";
import { useEffect, useMemo, useState } from "react";
import {
  type FactoryTopologyChromeConfiguration,
  type ResolvedFactoryTopologyChrome,
  resolveFactoryTopologyChrome,
} from "./factory-topology-chrome";
import { FactoryTopologyChromeRegions } from "./factory-topology-chrome-regions";
import {
  type FactoryTopologyFlowProjection,
  projectFactoryTopologyFlow,
} from "./factory-topology-flow-projection";
import {
  FactoryTopologyErrorBoundary,
  FactoryTopologyStateRegion,
  useDistinctTopologyErrorReport,
} from "./factory-topology-state";
import {
  normalizeFactoryVisualizerError,
  toFactoryVisualizerError,
} from "./visualizer-error";

export type { FactoryTopologyFlowProjection } from "./factory-topology-flow-projection";
export { projectFactoryTopologyFlow } from "./factory-topology-flow-projection";

import type { FactoryTopologyReplayError } from "./visualizer-error";

export interface FactoryTopologyReplayProjection {
  activity: FactoryActivityProjection;
  load: FactoryLoadProjection;
  topology: FactoryTopologyProjection;
}

export type FactoryTopologyReplayState =
  | { status: "empty" }
  | { status: "failed" }
  | { status: "loading" }
  | { projection: FactoryTopologyReplayProjection; status: "ready" };

export interface FactoryTopologyReplayMessages {
  activeDispatches: (count: number) => string;
  activeWorkDuration: (ticks: number) => string;
  activeWorkOverflow: (count: number) => string;
  activeWorkRows: (count: number) => string;
  annotationsHidden: string;
  annotationsVisible: string;
  empty: string;
  failed: string;
  hideNodeKinds: string;
  inactiveDispatches: string;
  imageFailed: string;
  imageLoading: string;
  legendLabel: string;
  loading: string;
  nodeLabel: (kind: FactoryTopologyNode["kind"], label: string) => string;
  regionLabel: string;
  resourceOccupancy: (occupied: number, capacity: number) => string;
  resourceOccupancyUnavailable: string;
  retry: string;
  selectedNode: string;
  showNodeKinds: string;
  workStateCount: (count: number) => string;
  workStateCountUnavailable: string;
}

export interface FactoryTopologyReplayProps {
  chrome?: FactoryTopologyChromeConfiguration;
  /** Presentation-only input validated against the prepared canonical topology. */
  layout?: unknown;
  messages: FactoryTopologyReplayMessages;
  onError?: (error: FactoryTopologyReplayError) => void;
  onRetry?: () => void;
  onSelectNode?: (node: FactoryTopologyNode) => void;
  selectedNodeId?: string;
  state: FactoryTopologyReplayState;
}

interface PreparedFlowSuccess {
  flow: FactoryTopologyFlowProjection;
  status: "ready";
}

interface PreparedFlowFailure {
  error: FactoryTopologyReplayError;
  status: "failed";
}

type PreparedFlow = PreparedFlowFailure | PreparedFlowSuccess;

export function FactoryTopologyReplay(props: FactoryTopologyReplayProps) {
  return (
    <FactoryTopologyErrorBoundary
      errorKind="render"
      messages={props.messages}
      onError={props.onError}
      onRetry={props.onRetry}
      resetKeys={[props.state, props.messages]}
    >
      <FactoryTopologyReplayContent {...props} />
    </FactoryTopologyErrorBoundary>
  );
}

function FactoryTopologyReplayContent({
  chrome,
  messages,
  onError,
  layout,
  onRetry,
  onSelectNode,
  selectedNodeId,
  state,
}: FactoryTopologyReplayProps) {
  if (state.status !== "ready") {
    return (
      <FactoryTopologyStateRegion
        messages={messages}
        state={state.status}
        onRetry={onRetry}
      />
    );
  }
  return (
    <PreparedTopology
      messages={messages}
      onError={onError}
      onRetry={onRetry}
      onSelectNode={onSelectNode}
      layout={layout}
      projection={state.projection}
      selectedNodeId={selectedNodeId}
      chrome={resolveFactoryTopologyChrome(chrome)}
    />
  );
}

function PreparedTopology({
  chrome,
  messages,
  onError,
  layout,
  onRetry,
  onSelectNode,
  projection,
  selectedNodeId,
}: Omit<FactoryTopologyReplayProps, "state"> & {
  chrome: ResolvedFactoryTopologyChrome;
  projection: FactoryTopologyReplayProjection;
}) {
  const prefersReducedMotion = usePrefersReducedMotion();
  const prepared = useMemo<PreparedFlow>(() => {
    try {
      const parsedLayout =
        layout === undefined
          ? undefined
          : safeParseFactoryVisualizationLayout(layout, {
              canonicalNodeIds: new Set(
                projection.topology.nodes.map((node) => node.id),
              ),
            });
      if (parsedLayout && !parsedLayout.success) {
        return {
          error: {
            issues: parsedLayout.issues.map(({ category, code, path }) => ({
              category,
              code,
              path,
            })),
            kind: "layout-validation",
            message: "The topology layout could not be prepared.",
            recoverable: true,
          },
          status: "failed",
        };
      }
      const flow = projectFactoryTopologyFlow(
        projection,
        messages,
        selectedNodeId,
        onSelectNode,
        prefersReducedMotion,
        parsedLayout?.data,
      );
      return flow.validEndpoints
        ? { flow, status: "ready" }
        : { error: toFactoryVisualizerError("endpoint"), status: "failed" };
    } catch (error) {
      return {
        error: normalizeFactoryVisualizerError(error, "projection"),
        status: "failed",
      };
    }
  }, [
    messages,
    onSelectNode,
    prefersReducedMotion,
    projection,
    layout,
    selectedNodeId,
  ]);
  useDistinctTopologyErrorReport(
    prepared.status === "failed" ? prepared.error : undefined,
    onError,
  );

  if (prepared.status === "failed") {
    return (
      <FactoryTopologyStateRegion
        messages={messages}
        state="failed"
        onRetry={onRetry}
      />
    );
  }

  return (
    <section
      aria-label={messages.regionLabel}
      className="factory-topology-replay"
      data-endpoints-valid="true"
      data-reduced-motion={prefersReducedMotion ? "true" : "false"}
    >
      <FactoryTopologyErrorBoundary
        errorKind="react-flow"
        messages={messages}
        onError={onError}
        onRetry={onRetry}
        resetKeys={[projection, messages, selectedNodeId, onSelectNode, layout]}
        withinRegion
      >
        <FactoryTopologyChromeRegions
          chrome={chrome}
          flow={prepared.flow}
          messages={messages}
        />
      </FactoryTopologyErrorBoundary>
    </section>
  );
}

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

function usePrefersReducedMotion(): boolean {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(
    () => reducedMotionMediaQuery()?.matches ?? false,
  );

  useEffect(() => {
    const mediaQuery = reducedMotionMediaQuery();
    if (!mediaQuery) return;

    const updatePreference = () => setPrefersReducedMotion(mediaQuery.matches);
    updatePreference();
    mediaQuery.addEventListener("change", updatePreference);
    return () => mediaQuery.removeEventListener("change", updatePreference);
  }, []);

  return prefersReducedMotion;
}

function reducedMotionMediaQuery(): MediaQueryList | undefined {
  return typeof window === "undefined" ||
    typeof window.matchMedia !== "function"
    ? undefined
    : window.matchMedia(REDUCED_MOTION_QUERY);
}
