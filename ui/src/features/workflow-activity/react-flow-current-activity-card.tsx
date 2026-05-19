import "@xyflow/react/dist/style.css";
import {
  Background,
  Controls,
  ReactFlow,
} from "@xyflow/react";
import type { CSSProperties } from "react";
import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import type { FactoryValue } from "../../api/named-factory";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/ui/dashboard-typography";
import { cx } from "../../lib/cx";
import {
  CURRENT_ACTIVITY_NODE_TYPES,
} from "../flowchart/current-activity-nodes";
import {
  FactoryImportPreviewDialog,
  type FactoryPngImportValue,
  type ReadFactoryImportFile,
} from "../import";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "./current-activity-import-controller";
import {
  DashboardFlowAxisLegend,
  getDefaultDashboardFlowAxisLegendEdgeItems,
  getDefaultDashboardFlowAxisLegendIconItems,
} from "./dashboard-flow-axis-legend";
import { EmptyCurrentActivityCard } from "./empty-current-activity-card";
import { getWorkflowActivityShellMessages } from "./messages/activity-shell";
import {
  GraphDropOverlay,
  GraphImportErrorPanel,
  graphDropStateAttribute,
} from "./react-flow-current-activity-card-import";
import { useCurrentActivityGraphViewModel } from "./react-flow-current-activity-card-view-model";

export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card-keys";

const GRAPH_BACKGROUND_COLOR = "var(--color-af-edge-muted-soft)";
const GRAPH_BACKGROUND_GAP = 24;
const GRAPH_BACKGROUND_SIZE = 1;

type CSSPropertiesWithVariables = CSSProperties &
  Record<`--${string}`, string | number>;

const GRAPH_CONTROLS_STYLE: CSSPropertiesWithVariables = {
  "--xy-controls-box-shadow": "none",
  "--xy-controls-button-background-color-hover-props":
    "rgb(from var(--color-af-overlay) r g b / 0.1)",
  "--xy-controls-button-background-color-props":
    "rgb(from var(--color-af-surface) r g b / 0.94)",
  "--xy-controls-button-border-color-props":
    "rgb(from var(--color-af-overlay) r g b / 0.08)",
  "--xy-controls-button-color-hover-props": "var(--color-af-ink)",
  "--xy-controls-button-color-props":
    "rgb(from var(--color-af-ink) r g b / 0.72)",
  backgroundColor: "rgb(from var(--color-af-surface) r g b / 0.88)",
  border: "1px solid rgb(from var(--color-af-overlay) r g b / 0.08)",
  borderRadius: 8,
  overflow: "hidden",
};

const CURRENT_ACTIVITY_CARD_CLASS =
  "relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-4 shadow-af-panel backdrop-blur-lg md:p-5";
const CURRENT_ACTIVITY_HEADER_CLASS = "mb-4";
const CURRENT_ACTIVITY_EYEBROW_CLASS =
  "mb-3 text-xs font-bold uppercase tracking-[0.16em] text-af-accent";
const CURRENT_ACTIVITY_LEGEND_CLASS =
  "absolute left-4 right-4 top-4 z-10 md:left-7 md:right-auto md:top-7";
const CURRENT_ACTIVITY_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);

export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };

function CurrentActivityCardHeading({ locale }: { locale?: string }) {
  const messages = getWorkflowActivityShellMessages(locale);

  return (
    <div>
      <p className={CURRENT_ACTIVITY_EYEBROW_CLASS}>{messages.eyebrow}</p>
      <h2 className={CURRENT_ACTIVITY_TITLE_CLASS} id="workflow-graph-heading">
        {messages.title}
      </h2>
    </div>
  );
}

interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  importController?: CurrentActivityImportController;
  locale?: string;
  now: number;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
  onSelectWorkstation: (nodeId: string) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}

export function ReactFlowCurrentActivityCard(
  props: ReactFlowCurrentActivityCardProps,
) {
  const graph = useCurrentActivityGraphViewModel(props);
  const fallbackImportController = useCurrentActivityImportController({
    activateFactory: props.activateFactory,
    onFactoryActivated: props.onFactoryActivated,
    onFactoryImportReady: props.onFactoryImportReady,
    readFactoryImportFile: props.readFactoryImportFile,
  });
  const imports = props.importController ?? fallbackImportController;
  const shouldRenderImportPreviewDialog = props.importController === undefined;

  if (props.snapshot.topology.workstation_node_ids.length === 0)
    return <EmptyCurrentActivityCard locale={props.locale} />;
  const shellMessages = getWorkflowActivityShellMessages(props.locale);

  const readyImportPreviewState =
    imports.importPreviewState.status === "ready"
      ? imports.importPreviewState
      : null;

  return (
    <section
      aria-labelledby="workflow-graph-heading"
      className={CURRENT_ACTIVITY_CARD_CLASS}
    >
      <div className={CURRENT_ACTIVITY_HEADER_CLASS}>
        <CurrentActivityCardHeading locale={props.locale} />
      </div>

      <div className="relative min-h-0 flex-1">
        <DashboardFlowAxisLegend
          className={CURRENT_ACTIVITY_LEGEND_CLASS}
          defaultExpanded={false}
          edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems(props.locale)}
          iconItems={getDefaultDashboardFlowAxisLegendIconItems(props.locale)}
          locale={props.locale}
        />
        <section
          aria-describedby="workflow-graph-heading"
          aria-label={shellMessages.viewportLabel}
          className={cx(
            "relative h-full min-h-0 overflow-hidden rounded-3xl border transition-colors",
            (imports.dropState.status === "drag-active" ||
              imports.dropState.status === "reading") &&
              "border-af-accent/35 bg-af-accent/6",
            imports.dropState.status === "error" && "border-af-danger/18",
            imports.dropState.status === "idle" && "border-transparent",
          )}
          data-current-activity-drop-state={graphDropStateAttribute(
            imports.dropState,
          )}
          data-current-activity-flow
          onDragEnter={imports.onDragEnter}
          onDragLeave={imports.onDragLeave}
          onDragOver={imports.onDragOver}
          onDrop={imports.onDrop}
        >
          <ReactFlow
            edges={graph.edges}
            fitView
            fitViewOptions={graph.initialFitViewOptions}
            key={graph.initialFitViewKey}
            maxZoom={2}
            minZoom={0.25}
            nodeTypes={CURRENT_ACTIVITY_NODE_TYPES}
            nodes={graph.nodes}
            nodesDraggable={true}
            onNodeDragStop={(_, node) => {
              if (graph.graphKey) {
                graph.setStoredNodePosition(
                  graph.graphKey,
                  node.id,
                  node.position,
                );
              }
            }}
            onNodesChange={graph.handleNodesChange}
            panOnDrag
            proOptions={{ hideAttribution: true }}
            zoomOnScroll
          >
            <Background
              color={GRAPH_BACKGROUND_COLOR}
              gap={GRAPH_BACKGROUND_GAP}
              size={GRAPH_BACKGROUND_SIZE}
            />
            <Controls
              fitViewOptions={{ maxZoom: 1.2, padding: 0.12 }}
              showInteractive={false}
              style={GRAPH_CONTROLS_STYLE}
            />
          </ReactFlow>
          <GraphDropOverlay
            dropState={imports.dropState}
            locale={props.locale}
          />
        </section>
      </div>
      {shouldRenderImportPreviewDialog && readyImportPreviewState ? (
        <FactoryImportPreviewDialog
          activationState={imports.activationState}
          locale={props.locale}
          onCancel={() => {
            imports.clearActivationError();
            imports.closeImportPreview();
          }}
          onConfirm={() => {
            void imports.activateImport(readyImportPreviewState.value);
          }}
          previewState={readyImportPreviewState}
        />
      ) : null}
      {imports.dropState.status === "error" ? (
        <GraphImportErrorPanel
          error={imports.dropState.error}
          fileName={imports.dropState.fileName}
          locale={props.locale}
          onDismiss={imports.clearError}
        />
      ) : null}
    </section>
  );
}
