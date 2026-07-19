import type { ReactFlowInstance } from "@xyflow/react";
import { factoryTopologyNodeId } from "@you-agent-factory/factory-replay";
import { useId, useMemo, useRef, useState } from "react";
import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  AlertPanelText,
  ExpandablePanelTrigger,
  SurfacePanel,
} from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { HostedTopologyReplay } from "../../dashboard/public";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { NODE_TYPES } from "../../flowchart/public";
import { FACTORY_GRAPH_EDGE_TYPES } from "../../graphs/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import {
  layoutValidationWarningMessages,
  mergeFactoryValidationTargets,
  saveErrorNoticeMessages,
  validationMessagesForGraphSelection,
} from "../lib/react-flow-current-activity-card-validation";
import {
  GraphDropOverlay,
  graphDropStateAttribute,
} from "./react-flow-current-activity-card-import";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

type CurrentActivityGraphSurfaceModel = CurrentActivityGraphCardViewModel;
type EditorNoticeTone = "danger" | "warning";
type EditorNoticeSection = {
  id: string;
  messages: readonly string[];
  title: string;
  tone: EditorNoticeTone;
};

const EDITOR_NOTICE_TONE_CLASS: Record<EditorNoticeTone, string> = {
  danger: "border-af-danger-border text-on-error-container",
  warning: "border-af-warning-border text-on-warning-container",
};

export function CurrentActivityGraphSurface({
  viewModel,
  headingID,
  imports,
  locale,
  onSelectResource,
  onSelectStateNode,
  onSelectWorker,
  onSelectWorkType,
  onSelectWorkstation,
  selection,
  snapshot: _snapshot,
}: {
  viewModel: CurrentActivityGraphCardViewModel;
  headingID: string;
  imports: CurrentActivityImportController;
  locale?: string;
  onSelectResource?: (resourceName: string) => void;
  onSelectStateNode?: (placeId: string) => void;
  onSelectWorker?: (workerName: string) => void;
  onSelectWorkType?: (workTypeName: string) => void;
  onSelectWorkstation?: (nodeId: string) => void;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}) {
  return (
    <CurrentActivityGraphSurfaceContent
      headingID={headingID}
      imports={imports}
      locale={locale}
      model={viewModel}
      onSelectResource={onSelectResource}
      onSelectStateNode={onSelectStateNode}
      onSelectWorker={onSelectWorker}
      onSelectWorkType={onSelectWorkType}
      onSelectWorkstation={onSelectWorkstation}
      selection={selection}
    />
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph surface keeps editor notices, validation, and viewport wiring together.
function CurrentActivityGraphSurfaceContent({
  headingID,
  imports,
  locale,
  model,
  onSelectResource,
  onSelectStateNode,
  onSelectWorker,
  onSelectWorkType,
  onSelectWorkstation,
  selection,
}: {
  headingID: string;
  imports: CurrentActivityImportController;
  locale?: string;
  model: CurrentActivityGraphSurfaceModel;
  onSelectResource?: (resourceName: string) => void;
  onSelectStateNode?: (placeId: string) => void;
  onSelectWorker?: (workerName: string) => void;
  onSelectWorkType?: (workTypeName: string) => void;
  onSelectWorkstation?: (nodeId: string) => void;
  selection: CurrentActivitySelection | null;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const flowContainerRef = useRef<HTMLElement | null>(null);
  const flowInstanceRef = useRef<ReactFlowInstance | null>(null);
  const saveError = model.status.saveError;
  const editorControls = model.editorControls;
  const removalControls = model.removalControls;
  const visualGroupControls = model.visualGroupControls;
  const editorValidationProjection = useMemo(() => {
    if (!editorControls.isEditing) {
      return model.validationControls.projection;
    }

    const saveErrorTargets =
      saveError instanceof CurrentFactoryDefinitionError
        ? (saveError.targets ?? [])
        : [];
    return mergeFactoryValidationTargets(
      model.validationControls.targets,
      saveErrorTargets,
    );
  }, [
    editorControls.isEditing,
    model.validationControls.projection,
    model.validationControls.targets,
    saveError,
  ]);
  const validationSelectionMessages = editorControls.isEditing
    ? validationMessagesForGraphSelection({
        factoryDefinition:
          model.validationControls.factoryDefinition ?? undefined,
        projection: editorValidationProjection,
        selectionNodeId:
          selection?.kind === "node" ? selection.nodeId : undefined,
        selectionPlaceId:
          selection?.kind === "state-node" ? selection.placeId : undefined,
      })
    : [];
  const draftValidationMessages = editorControls.isEditing
    ? model.validationControls.draftErrors
        .map((error) => error.message.trim())
        .filter((message) => message.length > 0)
    : [];
  const validationNoticeMessages = [
    ...new Set([...draftValidationMessages, ...validationSelectionMessages]),
  ];
  const layoutWarningMessages = editorControls.isEditing
    ? layoutValidationWarningMessages(model.validationControls.targets)
    : [];
  const saveFailureMessages = editorControls.isEditing
    ? saveErrorNoticeMessages(saveError)
    : [];
  const editorNoticeSections: EditorNoticeSection[] = [];
  if (saveFailureMessages.length > 0) {
    editorNoticeSections.push({
      id: "save-failure",
      messages: saveFailureMessages,
      title: messages.noticeSaveFailedTitle,
      tone: "danger",
    });
  }
  if (validationNoticeMessages.length > 0) {
    editorNoticeSections.push({
      id: "validation",
      messages: validationNoticeMessages,
      title: messages.noticeValidationFailureTitle,
      tone: "danger",
    });
  }
  if (layoutWarningMessages.length > 0) {
    editorNoticeSections.push({
      id: "layout-warning",
      messages: layoutWarningMessages,
      title: messages.noticeLayoutWarningTitle,
      tone: "warning",
    });
  }
  if (removalControls.blockedReason) {
    editorNoticeSections.push({
      id: "removal",
      messages: [removalControls.blockedReason],
      title: messages.noticeRemovalBlockedTitle,
      tone: "warning",
    });
  }
  if (editorControls.connectionNotice) {
    editorNoticeSections.push({
      id: "connection",
      messages: [editorControls.connectionNotice],
      title: messages.noticeConnectionBlockedTitle,
      tone: "warning",
    });
  }
  if (model.status.hasActiveWork && model.status.hasSharedGraphChanges) {
    editorNoticeSections.push({
      id: "active-work",
      messages: [messages.noticeTopologyBlockedDescription],
      title: messages.noticeTopologyBlockedTitle,
      tone: "danger",
    });
  }
  if (model.status.isStaleDraft) {
    editorNoticeSections.push({
      id: "stale-draft",
      messages: [messages.noticeStaleDescription],
      title: messages.noticeStaleTitle,
      tone: "warning",
    });
  }
  const layoutControls = model.layoutControls;
  const edgeWaypointControls = model.edgeWaypointControls;

  if (!editorControls.isEditing) {
    return (
      <section
        aria-label={messages.viewportLabel}
        className={cn(
          "relative min-h-0 min-w-0 flex-1 overflow-hidden rounded-3xl border transition-colors",
          (imports.dropState.status === "drag-active" ||
            imports.dropState.status === "reading") &&
            "border-primary bg-primary-container",
          imports.dropState.status === "error" && "border-af-danger-border",
          imports.dropState.status === "idle" && "border-transparent",
        )}
        data-current-activity-drop-state={graphDropStateAttribute(
          imports.dropState,
        )}
        onDragEnter={imports.onDragEnter}
        onDragLeave={imports.onDragLeave}
        onDragOver={imports.onDragOver}
        onDrop={imports.onDrop}
      >
        <HostedTopologyReplay
          locale={locale}
          onSelectResource={onSelectResource}
          onSelectStateNode={onSelectStateNode}
          onSelectWorker={onSelectWorker}
          onSelectWorkType={onSelectWorkType}
          onSelectWorkstation={onSelectWorkstation}
          selectedNodeID={hostedTopologySelectionID(selection)}
        />
        <GraphDropOverlay dropState={imports.dropState} locale={locale} />
      </section>
    );
  }

  return (
    <div
      className="relative grid max-h-full min-h-0 flex-1 overflow-hidden"
      style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
    >
      <CurrentActivityGraphViewport
        addControls={model.addControls}
        canDeleteGraphSelection={model.canDeleteGraphSelection}
        clearGraphSelection={model.clearGraphSelection}
        deleteGraphSelection={model.deleteGraphSelection}
        editorControls={editorControls}
        graphSelectionToolbarState={model.graphSelectionToolbarState}
        edgeTypes={FACTORY_GRAPH_EDGE_TYPES}
        edges={model.edges}
        flowContainerRef={flowContainerRef}
        flowInstanceRef={flowInstanceRef}
        handleEdgesChange={model.handleEdgesChange}
        handleGraphSelectionChange={model.handleGraphSelectionChange}
        handleGraphSelectionStart={model.handleGraphSelectionStart}
        handleNodesChange={model.handleNodesChange}
        hasPendingChanges={model.status.hasSharedGraphChanges}
        headingID={headingID}
        imports={imports}
        isSavingDraft={model.status.isSaving}
        layoutControls={layoutControls}
        locale={locale}
        nodeTypes={NODE_TYPES}
        nodes={model.nodes}
        visibilityControls={model.visibilityControls}
        onConnect={editorControls.connect}
        onEditorEdgeClick={edgeWaypointControls.handleEditorEdgeClick}
        onEditorEdgeDoubleClick={
          edgeWaypointControls.handleEditorEdgeDoubleClick
        }
        onMoveEdgeWaypoint={edgeWaypointControls.handleMoveSelectedEdgeWaypoint}
        onRemoveEdgeWaypoint={
          edgeWaypointControls.handleRemoveSelectedEdgeWaypoint
        }
        selectedEdgeWaypoints={edgeWaypointControls.selectedEdgeWaypoints}
        selectedWaypointEdgeId={edgeWaypointControls.selectedWaypointEdgeId}
        waypointAriaLabel={edgeWaypointControls.waypointAriaLabel}
        waypointControls={edgeWaypointControls.waypointControls}
        onCreateVisualGroup={visualGroupControls.handleCreateVisualGroup}
        onEditorNodeClick={(nodeId) => {
          visualGroupControls.clearSelectedVisualGroup();
          removalControls.deleteNode(nodeId);
        }}
        onMoveVisualGroup={visualGroupControls.handleMoveVisualGroup}
        onResizeVisualGroup={visualGroupControls.handleResizeVisualGroup}
        onSelectVisualGroup={visualGroupControls.handleSelectVisualGroup}
        selectedVisualGroupId={visualGroupControls.selectedGroupId}
        visualGroupCanEdit={visualGroupControls.canEditVisualGroups}
        visualGroupResizeHandleAriaLabel={
          visualGroupControls.resizeHandleAriaLabel
        }
        visualGroupControls={visualGroupControls.visualGroupControls}
        visualGroupAriaLabel={visualGroupControls.groupAriaLabel}
        visualGroups={visualGroupControls.groups}
        saveControls={model.saveControls}
        saveDisabledReason={model.status.saveBlockedReason}
      />
      {editorControls.isEditing ? (
        <CurrentActivityGraphEditorNoticePanel
          locale={locale}
          sections={editorNoticeSections}
        />
      ) : null}
    </div>
  );
}

function CurrentActivityGraphEditorNoticePanel({
  locale,
  sections,
}: {
  locale?: string;
  sections: readonly EditorNoticeSection[];
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const [expanded, setExpanded] = useState(true);
  const contentID = useId();
  const issueCount = sections.reduce(
    (total, section) => total + section.messages.length,
    0,
  );

  if (sections.length === 0) {
    return null;
  }

  return (
    <SurfacePanel
      asChild
      className={cn(
        // tailwind-exception: intrinsic-sizing
        "pointer-events-auto absolute right-4 top-4 z-30 max-h-[calc(100%-6rem)] overflow-hidden shadow-af-panel backdrop-blur-[16px]",
        // tailwind-exception: intrinsic-sizing
        expanded
          ? "w-[min(24rem,calc(100%-2rem))]"
          : // tailwind-exception: intrinsic-sizing
            "w-auto max-w-[calc(100%-2rem)]",
      )}
      padding="compact"
      radius="2xl"
    >
      <section
        aria-label={messages.noticePanelTitle}
        data-current-activity-editor-notice-panel=""
        role={
          sections.some((section) => section.tone === "danger")
            ? "alert"
            : "status"
        }
      >
        <div className="flex items-center justify-between gap-3 px-1 py-0.5">
          <div className="min-w-0">
            <h3 className="m-0 truncate text-sm font-semibold text-on-surface">
              {messages.noticePanelTitle}
            </h3>
            <p className="m-0 text-xs leading-5 text-on-surface-variant">
              {messages.noticePanelCount(issueCount)}
            </p>
          </div>
          <ExpandablePanelTrigger
            aria-label={
              expanded
                ? messages.noticePanelCollapseLabel
                : messages.noticePanelExpandLabel
            }
            className="h-9 w-9"
            controlsID={contentID}
            expanded={expanded}
            onClick={() => {
              setExpanded((current) => !current);
            }}
            variant="compact"
          />
        </div>
        {expanded ? (
          <div
            // tailwind-exception: intrinsic-sizing
            className="mt-2 max-h-[min(28rem,calc(100vh-14rem))] overflow-y-auto pr-1"
            id={contentID}
          >
            <div className="grid gap-3">
              {sections.map((section) => (
                <section
                  className={cn(
                    "grid gap-1.5 border-l-2 pl-3",
                    EDITOR_NOTICE_TONE_CLASS[section.tone],
                  )}
                  key={section.id}
                >
                  <h4 className="m-0 text-xs font-semibold uppercase leading-5 text-current">
                    {section.title}
                  </h4>
                  <AlertPanelText as="div">
                    <ul className="m-0 grid list-disc gap-1 pl-5">
                      {section.messages.map((message) => (
                        <li key={message}>{message}</li>
                      ))}
                    </ul>
                  </AlertPanelText>
                </section>
              ))}
            </div>
          </div>
        ) : null}
      </section>
    </SurfacePanel>
  );
}

function hostedTopologySelectionID(
  selection: CurrentActivitySelection | null,
): string | undefined {
  switch (selection?.kind) {
    case "node":
      return selection.nodeId.startsWith(
        factoryTopologyNodeId("workstation", ""),
      )
        ? selection.nodeId
        : factoryTopologyNodeId("workstation", selection.nodeId);
    case "resource":
      return factoryTopologyNodeId("resource", selection.resourceName);
    case "state-node":
      return factoryTopologyNodeId("work-state", selection.placeId);
    case "worker":
      return factoryTopologyNodeId("worker", selection.workerName);
    case "work-type":
      return factoryTopologyNodeId("work-type", selection.workTypeName);
    default:
      return undefined;
  }
}
