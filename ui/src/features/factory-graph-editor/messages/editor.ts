// biome-ignore-all lint/style/noExcessiveLinesPerFile: factory graph editor copy stays consolidated so message keys and locale fallbacks remain auditable during hardcoded-copy cleanup.

import type { components } from "../../../api/generated/openapi";
import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type {
  FactoryGraphNodeKind,
  FactoryWorkState,
} from "../lib/draft/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/editor/factory-graph-editor-additions";
import type {
  FactoryGraphEditorDirtyState,
  FactoryGraphSaveSummaryKind,
} from "../lib/editor-runtime/factory-graph-editor-dirty-state";
import type { FactoryGraphWorkerRuntimeStatus } from "../lib/editor-runtime/factory-graph-editor-runtime";

type ModelOperationContentType =
  components["schemas"]["ModelOperationContentType"];

export interface FactoryGraphEditorMessages {
  addDialogAddEntityAction: string;
  addDialogAssignedWorkerLabel: string;
  addDialogAssignedWorkerPlaceholder: string;
  addDialogCancelAction: string;
  addDialogCapacityLabel: string;
  addDialogDocContentHelp: string;
  addDialogDocContentLabel: string;
  addDialogDocFileNameHelp: string;
  addDialogDocFileNameLabel: string;
  addDialogDescription: (kind: FactoryGraphAddEntityDraft["kind"]) => string;
  addDialogFirstStateHelp: string;
  addDialogFirstStateLabel: string;
  addDialogIdentifierHelp: string;
  addDialogIdentifierLabel: string;
  addDialogKindLabel: string;
  addDialogModelHelp: string;
  addDialogModelLabel: string;
  addDialogModelOperationAddAction: string;
  addDialogModelOperationHeading: (operationIndex: number) => string;
  addDialogModelOperationInputsLabel: string;
  addDialogModelOperationNameHelp: string;
  addDialogModelOperationNameLabel: string;
  addDialogModelOperationOutputsLabel: string;
  addDialogModelOperationRemoveAction: string;
  addDialogModelOperationSlotAddAction: (
    direction: "input" | "output",
  ) => string;
  addDialogModelOperationSlotContentTypesLabel: string;
  addDialogModelOperationSlotHeading: (
    direction: "input" | "output",
    slotIndex: number,
  ) => string;
  addDialogModelOperationSlotNameLabel: string;
  addDialogModelOperationSlotRemoveAction: string;
  addDialogModelOperationSlotRequiredLabel: string;
  addDialogModelOperationsHelp: string;
  addDialogModelOperationsLabel: string;
  addDialogPromptBodyEditorError: string;
  addDialogPromptBodyEditorLoading: string;
  addDialogPromptBodyHelp: string;
  addDialogPromptBodyLabel: string;
  addDialogStateTypeLabel: string;
  addDialogTitle: (kind: FactoryGraphAddEntityDraft["kind"]) => string;
  addDialogWorkTypeLabel: string;
  addDialogWorkTypePlaceholder: string;
  addMenuAction: (kind: FactoryGraphAddEntityDraft["kind"]) => {
    description: string;
    label: string;
  };
  connectionAnchorDescription: (anchorId: string) => string;
  connectionAnchorLabel: (anchorId: string) => string;
  connectionFallbackNotice: string;
  connectionIncompatibleNotice: (
    sourceAnchor: string,
    sourceNode: string,
    targetAnchor: string,
    targetNode: string,
  ) => string;
  connectionSelectSourceNotice: string;
  defaultWorkTypeLabel: string;
  draftActionsAriaLabel: string;
  draftActionsDiscard: string;
  draftActionsSave: string;
  draftActionsSaving: string;
  draftActionsTitle: string;
  edgeAriaLabel: (label: string, source: string, target: string) => string;
  edgeWaypointAddLabel: string;
  edgeWaypointHandleLabel: (index: number) => string;
  edgeWaypointRemoveLabel: (index: number) => string;
  edgeWaypointKindLabel: string;
  edgeWaypointSelectedLabel: string;
  edgeWaypointSourceLabel: string;
  edgeWaypointTargetLabel: string;
  visualGroupAriaLabel: (group: { id: string; label?: string }) => string;
  visualGroupColorLabel: string;
  visualGroupColorOptionLabel: (
    token: "primary" | "info" | "success" | "warning" | "outline",
  ) => string;
  visualGroupEmptyLabelError: string;
  visualGroupInvalidBoundsError: string;
  visualGroupLabelFieldLabel: string;
  visualGroupMembershipEmptyLabel: string;
  visualGroupMembershipLabel: string;
  visualGroupMembershipNodeLabel: (label: string) => string;
  visualGroupMembershipStaleNodeLabel: (nodeId: string) => string;
  visualGroupSelectedLabel: string;
  visualGroupDeleteLabel: string;
  visualGroupResizeHandleLabel: (
    corner: "ne" | "nw" | "se" | "sw",
  ) => string;
  toolbarCreateGroupDescription: string;
  toolbarCreateGroupLabel: string;
  edgeKindLabel: (
    kind:
      | "worker-assignment"
      | "worker-resource"
      | "work-type-state"
      | "workstation-input"
      | "workstation-on-continue"
      | "workstation-on-failure"
      | "workstation-on-rejection"
      | "workstation-output"
      | "workstation-resource",
  ) => string;
  flowConnectionHint: string;
  flowPendingLabel: string;
  flowRemovingLabel: string;
  kindLabel: (kind: FactoryGraphNodeKind) => string;
  leaveDialogBody: string;
  leaveDialogDescription: string;
  leaveDialogKeepEditing: string;
  leaveDialogTitle: string;
  modeActive: string;
  modeClassifierRoutesUnavailable: (workstationName: string) => string;
  modeEnterEditor: string;
  modeLeaveEditor: string;
  modeLoadingDefinition: string;
  modeUnavailablePrefix: string;
  modeUnsavedChanges: string;
  modeObserve: string;
  noticeConnectionBlockedTitle: string;
  noticeEmptyMessage: string;
  noticeEmptyTitle: string;
  noticeDismissLabel: string;
  noticePanelCollapseLabel: string;
  noticePanelCount: (count: number) => string;
  noticePanelExpandLabel: string;
  noticePanelTitle: string;
  noticeRemovalBlockedTitle: string;
  noticeSaveFailedAffectedSummary: (labels: string) => string;
  noticeSaveFailedTitle: string;
  noticeSaveSuccessDescription: string;
  noticeSaveSuccessTitle: string;
  noticeStaleDescription: string;
  noticeStaleTitle: string;
  noticeTopologyBlockedDescription: string;
  noticeTopologyBlockedTitle: string;
  noticeLayoutWarningTitle: string;
  noticeValidationFailureTitle: string;
  operationConnectionInvalid: string;
  operationEdgeNotFound: (edgeId: string) => string;
  operationGraphEditsInvalid: string;
  operationNodeNotFound: (nodeId: string) => string;
  saveConfirmAction: (kind: FactoryGraphSaveSummaryKind) => string;
  saveBlockedActiveWork: string;
  saveBlockedStaleDraft: string;
  saveConfirmTitle: string;
  dirtyStateSummary: (dirtyState: FactoryGraphEditorDirtyState) => string;
  saveSummaryDescription: (summary: {
    changedEdges: number;
    createdEntities: number;
    removedEntities: number;
  }) => string;
  saveSummaryForDirtyState: (summary: {
    changedEdges: number;
    createdEntities: number;
    dirtyState: FactoryGraphEditorDirtyState;
    kind: FactoryGraphSaveSummaryKind;
    removedEntities: number;
    topologySummary: string;
  }) => string;
  stateCollapsed: string;
  stateTypeLabel: (stateType: FactoryWorkState["type"]) => string;
  stateVisible: string;
  toolbarAddDescription: string;
  toolbarAddLabel: string;
  toolbarAriaLabel: string;
  toolbarConnectDescription: string;
  toolbarConnectLabel: string;
  toolbarDeleteDescription: string;
  toolbarDeleteDisabledNoSelectionDescription: string;
  toolbarDeleteDisabledNoSelectionLabel: string;
  toolbarDeleteDisabledNonDeletableDescription: string;
  toolbarDeleteDisabledNonDeletableLabel: string;
  toolbarDeleteLabel: string;
  toolbarDeleteMultiSelectionDescription: (count: number) => string;
  toolbarDeleteMultiSelectionLabel: (count: number) => string;
  toolbarDeleteSelectionDescription: string;
  toolbarDeleteSingleSelectionDescription: string;
  toolbarDeleteSingleSelectionLabel: string;
  toolbarRedoDescription: string;
  toolbarRedoLabel: string;
  toolbarResetLayoutDescription: string;
  toolbarResetLayoutLabel: string;
  toolbarUndoDescription: string;
  toolbarUndoLabel: string;
  toolbarHideShowDescription: string;
  toolbarHideShowLabel: string;
  toolbarHideShowMenuAriaLabel: string;
  toolbarHideShowMenuDescription: string;
  toolbarHideShowMenuTitle: string;
  toolbarClearPreferencesDescription: string;
  toolbarClearPreferencesLabel: string;
  toolbarOpenAddMenuLabel: string;
  toolbarOpenHideShowMenuLabel: string;
  nodeClassVisibilityDescription: (kind: FactoryGraphNodeKind) => string;
  toolbarVisibilityMenuAriaLabel: string;
  toolbarVisibilityMenuDescription: string;
  toolbarVisibilityMenuTitle: string;
  visibilityPresetAllLabel: string;
  visibilityPresetExecutionLabel: string;
  visibilityPresetInfrastructureLabel: string;
  visibilityPresetWorkflowLabel: string;
  visibilityPresetsAriaLabel: string;
  viewportLabel: string;
  validationDuplicateIdentifier: (nodeId: string) => string;
  validationIncompatibleEdge: (
    relationship: string,
    source: string,
    target: string,
  ) => string;
  validationMissingRequiredIdentifier: (kind: FactoryGraphNodeKind) => string;
  validationMissingWorkerAssignment: (workstationName: string) => string;
  validationUnknownWorkstationRoute: (input: {
    routeField: string;
    stateName: string;
    workstationName: string;
    workTypeName: string;
  }) => string;
  validationUnknownEdgeNode: (
    relationship: string,
    which: "source" | "target",
  ) => string;
  removalDescription: (input: {
    connectedEdgeCount: number;
    impactedStateCount: number;
    kind: FactoryGraphNodeKind;
    label: string;
  }) => string;
  removalEdgeConfirmLabel: (edgeLabel: string) => string;
  removalEdgeDescription: (
    kind: Parameters<FactoryGraphEditorMessages["edgeKindLabel"]>[0],
    source: string,
    target: string,
  ) => string;
  removalEdgeIneligibleWorkTypeState: string;
  removalEdgeLabel: (
    kind: Parameters<FactoryGraphEditorMessages["edgeKindLabel"]>[0],
    source: string,
  ) => string;
  removalEdgeTitle: (edgeLabel: string) => string;
  removalEntityConfirmLabel: (
    label: string,
    kind: FactoryGraphNodeKind,
  ) => string;
  removalEntityTitle: (label: string, kind: FactoryGraphNodeKind) => string;
  removalDocConfirmLabel: (displayLabel: string) => string;
  removalDocDescription: (targetPath: string) => string;
  removalDocTitle: (displayLabel: string) => string;
  removalBatchConfirmLabel: (itemCount: number) => string;
  removalBatchDescription: (itemCount: number) => string;
  removalBatchTitle: (itemCount: number) => string;
  removalFallbackConfirmDescription: string;
  removalFallbackConfirmLabel: string;
  removalFallbackTitle: string;
  removalWorkerAssignedReason: (
    workstationCount: number,
    workerLabel: string,
  ) => string;
  localizeModelOperationContentType: (
    contentType: ModelOperationContentType,
  ) => string;
  workerStatusLabel: (status: FactoryGraphWorkerRuntimeStatus) => string;
  workStatePhaseLegendAriaLabel: string;
  workStatePhaseLegendLabel: (stateType: FactoryWorkState["type"]) => string;
  zAxisIncompleteConnectionHint: string;
}

function describeEnglishAddDialog(kind: FactoryGraphAddEntityDraft["kind"]) {
  if (kind === "doc") {
    return "Create a bundled doc under factory/docs with editable text before save.";
  }
  if (kind === "workstation") {
    return "Create a pending workstation in the current graph draft.";
  }
  if (kind === "work-type") {
    return "Define a new work type and its first ordered state.";
  }
  if (kind === "work-state") {
    return "Append a new ordered state to an existing work type.";
  }
  if (kind === "worker") {
    return "Create a pending model or script worker in the current graph draft.";
  }
  return `Create a pending ${kind} in the current graph draft.`;
}

function describeEnglishAddDialogTitle(
  kind: FactoryGraphAddEntityDraft["kind"],
) {
  if (kind === "doc") {
    return "Add doc";
  }
  if (kind === "work-type") {
    return "Add work type";
  }
  if (kind === "work-state") {
    return "Add work state";
  }
  return `Add ${kind}`;
}

function describeEnglishKind(kind: FactoryGraphNodeKind) {
  switch (kind) {
    case "doc":
      return "Doc";
    case "resource":
      return "Resource";
    case "worker":
      return "Worker";
    case "workstation":
      return "Workstation";
    case "work-type":
      return "Work type";
    case "work-state":
      return "Work state";
  }
}

function describeEnglishNodeClassVisibility(kind: FactoryGraphNodeKind) {
  return `Show ${describeEnglishKind(kind).toLowerCase()} nodes on the graph.`;
}

function describeEnglishWorkerStatus(status: FactoryGraphWorkerRuntimeStatus) {
  switch (status) {
    case "active":
      return "Active";
    case "errored":
      return "Errored";
    case "idle":
      return "Idle";
    case "unavailable":
      return "Unavailable";
  }
}

function describeEnglishEdgeKind(
  kind: Parameters<FactoryGraphEditorMessages["edgeKindLabel"]>[0],
) {
  switch (kind) {
    case "worker-assignment":
      return "Worker assignment";
    case "worker-resource":
      return "Worker resource";
    case "work-type-state":
      return "State membership";
    case "workstation-input":
      return "Input route";
    case "workstation-on-continue":
      return "Continue route";
    case "workstation-on-failure":
      return "Failure route";
    case "workstation-on-rejection":
      return "Reject route";
    case "workstation-output":
      return "Success route";
    case "workstation-resource":
      return "Station resource";
  }
}

function pluralizeEnglish(count: number, noun: string) {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

function describeEnglishCount(count: number, singular: string) {
  if (count === 0) {
    return null;
  }
  const plural =
    singular === "created entity" || singular === "deleted entity"
      ? `${singular.slice(0, -1)}ies`
      : `${singular}s`;
  return `${count} ${count === 1 ? singular : plural}`;
}

function describeEnglishSaveSummary(summary: {
  changedEdges: number;
  createdEntities: number;
  removedEntities: number;
}) {
  const segments = [
    describeEnglishCount(summary.createdEntities, "created entity"),
    describeEnglishCount(summary.removedEntities, "deleted entity"),
    describeEnglishCount(summary.changedEdges, "changed edge"),
  ].filter((segment) => segment !== null);

  if (segments.length === 0) {
    return "No graph topology changes are pending.";
  }
  if (segments.length === 1) {
    return `This save will apply ${segments[0]}.`;
  }

  const finalSegment = segments[segments.length - 1];
  return `This save will apply ${segments.slice(0, -1).join(", ")} and ${finalSegment}.`;
}

function describeEnglishSaveConfirmAction(kind: FactoryGraphSaveSummaryKind) {
  switch (kind) {
    case "layout-only":
      return "Save layout";
    case "topology-only":
      return "Save topology";
    case "mixed":
      return "Save changes";
    case "preferences-only":
      return "Save preferences";
    case "none":
      return "Save changes";
  }
}

function describeEnglishDirtyStateSummary(
  dirtyState: FactoryGraphEditorDirtyState,
) {
  if (
    dirtyState.preferencesDirty &&
    !dirtyState.layoutDirty &&
    !dirtyState.topologyDirty
  ) {
    return "Private view preferences changed";
  }
  if (dirtyState.layoutDirty && dirtyState.topologyDirty) {
    return "Unsaved layout and topology changes";
  }
  if (dirtyState.layoutDirty) {
    return "Unsaved layout changes";
  }
  if (dirtyState.topologyDirty) {
    return "Unsaved topology changes";
  }
  return "Unsaved changes";
}

function describeEnglishSaveSummaryForDirtyState(summary: {
  changedEdges: number;
  createdEntities: number;
  dirtyState: FactoryGraphEditorDirtyState;
  kind: FactoryGraphSaveSummaryKind;
  removedEntities: number;
  topologySummary: string;
}) {
  switch (summary.kind) {
    case "preferences-only":
      return "Visibility and filter preferences changed for your view only. They stay private and are not saved into the shared factory document.";
    case "layout-only":
      return "This save will update shared graph layout, visual groups, and viewport. Factory topology stays unchanged.";
    case "topology-only":
      return summary.topologySummary;
    case "mixed":
      return `This save will update shared graph layout and apply topology changes: ${summary.topologySummary.replace(/^This save will apply /, "").replace(/\.$/, "")}.`;
    case "none":
      return "No shared factory document changes are pending.";
  }
}

function describeEnglishAddMenuAction(
  kind: FactoryGraphAddEntityDraft["kind"],
) {
  switch (kind) {
    case "doc":
      return {
        description:
          "Add a bundled documentation file under factory/docs with editable text.",
        label: "Doc",
      };
    case "workstation":
      return {
        description:
          "Create a pending workstation and assign an existing worker.",
        label: "Workstation",
      };
    case "worker":
      return {
        description:
          "Add a model or script worker that can be assigned to workstations.",
        label: "Worker",
      };
    case "work-type":
      return {
        description: "Define a new work type with its first ordered state.",
        label: "Work type",
      };
    case "work-state":
      return {
        description: "Append a state to an existing work type.",
        label: "Work state",
      };
    case "resource":
      return {
        description:
          "Register a resource that workers or workstations can consume.",
        label: "Resource",
      };
  }
}

function describeEnglishConnectionAnchor(anchorId: string) {
  switch (anchorId) {
    case "worker-resource-source":
      return {
        description: "Provide this resource to a worker.",
        label: "Worker",
      };
    case "workstation-resource-source":
      return {
        description: "Provide this resource to a workstation.",
        label: "Station",
      };
    case "worker-input-target":
      return {
        description: "Accept a resource required by this worker.",
        label: "Resource",
      };
    case "worker-assignment-source":
      return {
        description: "Assign this worker to a workstation.",
        label: "Assign",
      };
    case "workstation-input-source":
    case "workstation-input-target":
      return {
        description:
          anchorId === "workstation-input-source"
            ? "Route this work state into a workstation input."
            : "Accept an input work state for this workstation.",
        label: "Input",
      };
    case "workstation-output-source":
      return {
        description: "Route successful output from this workstation.",
        label: "Success",
      };
    case "workstation-on-continue-source":
      return {
        description: "Route a continue transition from this workstation.",
        label: "Continue",
      };
    case "workstation-on-failure-source":
      return {
        description: "Route a failure transition from this workstation.",
        label: "Failure",
      };
    case "workstation-on-rejection-source":
      return {
        description: "Route a rejection transition from this workstation.",
        label: "Reject",
      };
    case "work-state-input-target":
      return {
        description: "Receive workstation output into this work state.",
        label: "Input",
      };
    case "worker-assignment-target":
      return {
        description: "Accept a worker assignment for this workstation.",
        label: "Worker",
      };
    case "workstation-resource-target":
      return {
        description: "Accept a resource requirement for this workstation.",
        label: "Resource",
      };
    default:
      return { description: "", label: "" };
  }
}

function describeEnglishStateType(stateType: FactoryWorkState["type"]) {
  return stateType;
}

function describeEnglishModelOperationContentType(
  contentType: ModelOperationContentType,
) {
  switch (contentType) {
    case "TEXT":
      return "Text";
    case "IMAGE":
      return "Image";
    case "AUDIO":
      return "Audio";
    case "JSON":
      return "JSON";
    case "BINARY":
      return "Binary";
  }
}

function describeEnglishWorkStatePhaseLegendLabel(
  stateType: FactoryWorkState["type"],
) {
  switch (stateType) {
    case "INITIAL":
      return "Initial";
    case "PROCESSING":
      return "Processing";
    case "TERMINAL":
      return "Completed";
    case "FAILED":
      return "Failed";
  }
}

const factoryGraphEditorMessagesByLocale: LocalizedMessageCatalog<FactoryGraphEditorMessages> =
  {
    en: {
      addDialogAddEntityAction: "Add entity",
      addDialogAssignedWorkerLabel: "Assigned worker",
      addDialogAssignedWorkerPlaceholder: "Select a worker",
      addDialogCancelAction: "Cancel",
      addDialogCapacityLabel: "Capacity",
      addDialogDocContentHelp:
        "Optional UTF-8 text saved with the bundled doc entry.",
      addDialogDocContentLabel: "Doc text",
      addDialogDocFileNameHelp:
        "Saved under factory/docs/. Include the file extension.",
      addDialogDocFileNameLabel: "File name",
      addDialogDescription: describeEnglishAddDialog,
      addDialogFirstStateHelp:
        "New work types start with one required ordered state.",
      addDialogFirstStateLabel: "First state",
      addDialogIdentifierHelp:
        "Use the authored name the factory definition should save.",
      addDialogIdentifierLabel: "Identifier",
      addDialogKindLabel: "Kind",
      addDialogModelHelp:
        "The model identifier saved on the new inference worker.",
      addDialogModelLabel: "Model",
      addDialogModelOperationAddAction: "Add operation",
      addDialogModelOperationHeading: (operationIndex) =>
        `Operation ${operationIndex + 1}`,
      addDialogModelOperationInputsLabel: "Input slots",
      addDialogModelOperationNameHelp:
        "Use uppercase names such as TTS or ASR.",
      addDialogModelOperationNameLabel: "Operation name",
      addDialogModelOperationOutputsLabel: "Output slots",
      addDialogModelOperationRemoveAction: "Remove operation",
      addDialogModelOperationSlotAddAction: (direction) =>
        direction === "input" ? "Add input slot" : "Add output slot",
      addDialogModelOperationSlotContentTypesLabel: "Content types",
      addDialogModelOperationSlotHeading: (direction, slotIndex) =>
        `${direction === "input" ? "Input" : "Output"} slot ${slotIndex + 1}`,
      addDialogModelOperationSlotNameLabel: "Slot name",
      addDialogModelOperationSlotRemoveAction: "Remove slot",
      addDialogModelOperationSlotRequiredLabel: "Required input slot",
      addDialogModelOperationsHelp:
        "Optionally declare provider-agnostic operations with typed input and output slots for inference-run workstations.",
      addDialogModelOperationsLabel: "Model operations (optional)",
      addDialogPromptBodyEditorError:
        "The prompt editor could not start. Edit the prompt text below while we recover.",
      addDialogPromptBodyEditorLoading: "Starting the prompt editor.",
      addDialogPromptBodyHelp:
        "Optional prompt content for the workstation body.",
      addDialogPromptBodyLabel: "Prompt body",
      addDialogStateTypeLabel: "State type",
      addDialogTitle: describeEnglishAddDialogTitle,
      addDialogWorkTypeLabel: "Work type",
      addDialogWorkTypePlaceholder: "Select a work type",
      addMenuAction: describeEnglishAddMenuAction,
      connectionAnchorDescription: (anchorId) =>
        describeEnglishConnectionAnchor(anchorId).description,
      connectionAnchorLabel: (anchorId) =>
        describeEnglishConnectionAnchor(anchorId).label,
      connectionFallbackNotice:
        "Choose a compatible source and target anchor before creating a connection.",
      connectionIncompatibleNotice: (
        sourceAnchor,
        sourceNode,
        targetAnchor,
        targetNode,
      ) =>
        `${sourceAnchor} connections from ${sourceNode} cannot connect to ${targetAnchor} on ${targetNode}.`,
      connectionSelectSourceNotice:
        "Select a source anchor before choosing a target anchor.",
      defaultWorkTypeLabel: "Default work type",
      draftActionsAriaLabel: "Pending graph changes",
      draftActionsDiscard: "Discard changes",
      draftActionsSave: "Save changes",
      draftActionsSaving: "Saving...",
      draftActionsTitle: "Pending graph changes",
      edgeAriaLabel: (label, source, target) =>
        `${label} from ${source} to ${target}`,
      edgeWaypointAddLabel: "Add waypoint",
      edgeWaypointHandleLabel: (index) => `Move edge waypoint ${index + 1}`,
      edgeWaypointRemoveLabel: (index) => `Remove waypoint ${index + 1}`,
      edgeWaypointKindLabel: "Kind",
      edgeWaypointSelectedLabel: "Selected edge route",
      edgeWaypointSourceLabel: "Source",
      edgeWaypointTargetLabel: "Target",
      visualGroupAriaLabel: (group) =>
        group.label?.trim()
          ? `Visual group ${group.label.trim()}`
          : `Visual group ${group.id}`,
      visualGroupColorLabel: "Group color",
      visualGroupColorOptionLabel: (token) => `Use ${token} group color`,
      visualGroupEmptyLabelError: "Enter a group label.",
      visualGroupInvalidBoundsError:
        "Group bounds contain non-finite geometry. Resize the group to correct them before saving.",
      visualGroupLabelFieldLabel: "Group label",
      visualGroupMembershipEmptyLabel: "No canvas nodes are available to assign.",
      visualGroupMembershipLabel: "Group members",
      visualGroupMembershipNodeLabel: (label) => `Include ${label} in this group`,
      visualGroupMembershipStaleNodeLabel: (nodeId) =>
        `Saved member ${nodeId} is no longer on the canvas.`,
      visualGroupSelectedLabel: "Selected visual group",
      visualGroupDeleteLabel: "Delete group",
      visualGroupResizeHandleLabel: (corner) =>
        `Resize group from ${corner} corner`,
      toolbarCreateGroupDescription: "Create a labeled background group",
      toolbarCreateGroupLabel: "Create group",
      edgeKindLabel: describeEnglishEdgeKind,
      flowConnectionHint: "Use labeled anchors for compatible connections.",
      flowPendingLabel: "Pending",
      flowRemovingLabel: "Removing",
      kindLabel: describeEnglishKind,
      leaveDialogBody:
        "Save to keep the pending factory topology, discard to revert to the latest server-backed graph, or keep editing.",
      leaveDialogDescription:
        "This graph editor session still has local topology changes.",
      leaveDialogKeepEditing: "Keep editing",
      leaveDialogTitle: "Leave graph editor with unsaved changes?",
      modeActive: "Editor mode active",
      modeClassifierRoutesUnavailable: (workstationName) =>
        `Factory graph editing does not yet support classifier workstation routes. "${workstationName}" stays read-only in this view until labeled route editing is available.`,
      modeEnterEditor: "Edit mode",
      modeLeaveEditor: "Leave editor",
      modeLoadingDefinition: "Loading editor definition",
      modeUnavailablePrefix: "Editor unavailable",
      modeUnsavedChanges: "Unsaved changes",
      modeObserve: "Observe",
      noticeConnectionBlockedTitle: "Connection blocked",
      noticeEmptyMessage:
        "The factory has not published any workstation graph yet.",
      noticeEmptyTitle: "No workflow topology loaded",
      noticeDismissLabel: "Dismiss",
      noticePanelCollapseLabel: "Collapse editor alerts",
      noticePanelCount: (count) =>
        count === 1 ? "1 issue" : `${count} issues`,
      noticePanelExpandLabel: "Expand editor alerts",
      noticePanelTitle: "Editor alerts",
      noticeRemovalBlockedTitle: "Removal blocked",
      noticeSaveFailedAffectedSummary: (labels) => `Affected: ${labels}`,
      noticeSaveFailedTitle: "Topology save failed",
      noticeSaveSuccessDescription:
        "The draft has been cleared and the graph is waiting for the latest factory-change event refresh.",
      noticeSaveSuccessTitle: "Topology saved",
      noticeStaleDescription:
        "Refresh or discard the current draft before saving so you do not overwrite a newer topology version.",
      noticeStaleTitle: "A newer factory definition is available",
      noticeTopologyBlockedDescription:
        "Save is unavailable while active work is still running in the current factory.",
      noticeTopologyBlockedTitle: "Topology edits are blocked",
      noticeLayoutWarningTitle: "Recoverable layout warning",
      noticeValidationFailureTitle: "Factory validation issue",
      operationConnectionInvalid: "Graph connection is invalid.",
      operationEdgeNotFound: (edgeId) =>
        `Graph edge "${edgeId}" was not found.`,
      operationGraphEditsInvalid:
        "Graph edits must be valid before they can be applied.",
      operationNodeNotFound: (nodeId) =>
        `Graph node "${nodeId}" was not found.`,
      dirtyStateSummary: describeEnglishDirtyStateSummary,
      saveConfirmAction: describeEnglishSaveConfirmAction,
      saveBlockedActiveWork:
        "Topology save is unavailable while active work is still running in this factory.",
      saveBlockedStaleDraft:
        "A newer factory topology arrived while this draft was open. Refresh or discard before saving.",
      saveConfirmTitle: "Save factory graph changes?",
      saveSummaryDescription: describeEnglishSaveSummary,
      saveSummaryForDirtyState: describeEnglishSaveSummaryForDirtyState,
      stateCollapsed: "Collapsed",
      stateTypeLabel: describeEnglishStateType,
      stateVisible: "Visible",
      toolbarAddDescription: "Add",
      toolbarAddLabel: "Add",
      toolbarAriaLabel: "Factory graph editor tools",
      toolbarConnectDescription: "Connect",
      toolbarConnectLabel: "Connect",
      toolbarDeleteDescription: "Remove",
      toolbarDeleteDisabledNoSelectionDescription:
        "Select graph items to delete",
      toolbarDeleteDisabledNoSelectionLabel: "Delete, no graph items selected",
      toolbarDeleteDisabledNonDeletableDescription:
        "Selected graph items cannot be removed",
      toolbarDeleteDisabledNonDeletableLabel:
        "Delete, selected items cannot be removed",
      toolbarDeleteLabel: "Delete",
      toolbarDeleteMultiSelectionDescription: (count) =>
        `Delete ${count} selected graph items`,
      toolbarDeleteMultiSelectionLabel: (count) =>
        `Delete ${count} selected graph items`,
      toolbarDeleteSelectionDescription: "Delete selected graph items",
      toolbarDeleteSingleSelectionDescription: "Delete selected graph item",
      toolbarDeleteSingleSelectionLabel: "Delete selected graph item",
      toolbarRedoDescription: "Redo the last undone layout change",
      toolbarRedoLabel: "Redo",
      toolbarResetLayoutDescription:
        "Reset node positions to the saved shared layout baseline",
      toolbarResetLayoutLabel: "Reset layout",
      toolbarUndoDescription: "Undo the last layout change",
      toolbarUndoLabel: "Undo",
      toolbarHideShowDescription: "Show",
      toolbarHideShowLabel: "Show or hide",
      toolbarHideShowMenuAriaLabel: "Factory graph node class visibility menu",
      toolbarHideShowMenuDescription:
        "Toggle which node classes appear on the graph. Hidden classes stay out of the view until you show them again.",
      toolbarHideShowMenuTitle: "Hide or show node classes",
      toolbarClearPreferencesDescription:
        "Restore the shared authored graph view for this factory. Private visibility and filter choices are cleared.",
      toolbarClearPreferencesLabel: "Clear private view preferences",
      toolbarOpenAddMenuLabel: "Add",
      toolbarOpenHideShowMenuLabel: "Show or hide",
      nodeClassVisibilityDescription: describeEnglishNodeClassVisibility,
      toolbarVisibilityMenuAriaLabel: "Add graph entity menu",
      toolbarVisibilityMenuDescription:
        "Choose a supported entity to add to the current draft.",
      toolbarVisibilityMenuTitle: "Add graph entity",
      visibilityPresetAllLabel: "All",
      visibilityPresetExecutionLabel: "Execution",
      visibilityPresetInfrastructureLabel: "Infrastructure",
      visibilityPresetWorkflowLabel: "Workflow",
      visibilityPresetsAriaLabel: "Factory graph visibility presets",
      viewportLabel: "Work graph viewport",
      validationDuplicateIdentifier: (nodeId) =>
        `Duplicate graph identifier "${nodeId}" is not allowed.`,
      validationIncompatibleEdge: (relationship, source, target) =>
        `Relationship "${relationship}" cannot connect ${source} to ${target}.`,
      validationMissingRequiredIdentifier: (kind) =>
        `${kind} identifiers are required before save.`,
      validationMissingWorkerAssignment: (workstationName) =>
        `Workstation "${workstationName}" must keep one worker assignment.`,
      validationUnknownWorkstationRoute: ({
        routeField,
        stateName,
        workstationName,
        workTypeName,
      }) =>
        `Workstation "${workstationName}" ${routeField} references unknown work state "${workTypeName}:${stateName}".`,
      validationUnknownEdgeNode: (relationship, which) =>
        `Relationship "${relationship}" references an unknown ${which} node.`,
      removalDescription: ({
        connectedEdgeCount,
        impactedStateCount,
        kind,
        label,
      }) => {
        const edgeSummary =
          connectedEdgeCount > 0
            ? `This will remove ${pluralizeEnglish(connectedEdgeCount, "graph edge")}.`
            : "This entity has no connected graph edges to remove.";

        if (kind === "work-type") {
          return `${edgeSummary} ${label} also owns ${pluralizeEnglish(
            impactedStateCount,
            "work state",
          )}, which will be removed with it.`;
        }
        if (kind === "work-state") {
          return `${edgeSummary} Any workstation routes that reference ${label} will be cleared from the pending draft.`;
        }
        if (kind === "resource") {
          return `${edgeSummary} Worker and workstation resource references that depend on ${label} will be cleared from the pending draft.`;
        }
        return edgeSummary;
      },
      removalEdgeConfirmLabel: (edgeLabel) => `Remove ${edgeLabel}`,
      removalEdgeDescription: (kind, source, target) => {
        switch (kind) {
          case "worker-assignment":
            return `This will unassign ${source} from ${target}. The workstation will need another worker before topology save can succeed.`;
          case "worker-resource":
            return `This will remove ${source} from ${target}'s available resources in the pending draft.`;
          case "workstation-resource":
            return `This will remove ${source} from ${target}'s required resources in the pending draft.`;
          case "workstation-input":
            return `This will stop routing ${source} into ${target}.`;
          case "workstation-output":
            return `This will remove the success route from ${source} to ${target}.`;
          case "workstation-on-continue":
            return `This will remove the continue route from ${source} to ${target}.`;
          case "workstation-on-failure":
            return `This will remove the failure route from ${source} to ${target}.`;
          case "workstation-on-rejection":
            return `This will remove the rejection route from ${source} to ${target}.`;
          case "work-type-state":
            return "";
        }
      },
      removalEdgeIneligibleWorkTypeState:
        "Work type ordering edges are managed by work-state membership and cannot be removed directly.",
      removalEdgeLabel: (kind, source) => {
        switch (kind) {
          case "worker-assignment":
            return `${source} assignment`;
          case "worker-resource":
          case "workstation-resource":
            return `${source} resource link`;
          case "workstation-input":
            return `${source} input route`;
          case "workstation-output":
            return `${source} success route`;
          case "workstation-on-continue":
            return `${source} continue route`;
          case "workstation-on-failure":
            return `${source} failure route`;
          case "workstation-on-rejection":
            return `${source} rejection route`;
          case "work-type-state":
            return `${source} state membership`;
        }
      },
      removalEdgeTitle: (edgeLabel) => `Remove ${edgeLabel}?`,
      removalEntityConfirmLabel: (label, kind) => `Delete ${label} ${kind}`,
      removalEntityTitle: (label, kind) => `Remove ${label} ${kind}?`,
      removalDocConfirmLabel: (displayLabel) => `Delete ${displayLabel} doc`,
      removalDocDescription: (targetPath) =>
        `This removes the bundled doc at ${targetPath} from the current factory draft.`,
      removalDocTitle: (displayLabel) => `Remove ${displayLabel} doc?`,
      removalBatchConfirmLabel: (itemCount) =>
        `Delete ${itemCount} selected graph items`,
      removalBatchDescription: (itemCount) =>
        `This removes ${itemCount} selected graph items from the current draft.`,
      removalBatchTitle: (itemCount) =>
        `Remove ${itemCount} selected graph items?`,
      removalFallbackConfirmDescription:
        "Remove this graph entity from the current draft.",
      removalFallbackConfirmLabel: "Delete entity",
      removalFallbackTitle: "Remove graph entity?",
      removalWorkerAssignedReason: (workstationCount, workerLabel) =>
        `This worker is still assigned to ${pluralizeEnglish(
          workstationCount,
          "workstation",
        )}. Reassign or remove those workstations before deleting ${workerLabel}.`,
      localizeModelOperationContentType:
        describeEnglishModelOperationContentType,
      workerStatusLabel: describeEnglishWorkerStatus,
      workStatePhaseLegendAriaLabel: "Work state lifecycle colors",
      workStatePhaseLegendLabel: describeEnglishWorkStatePhaseLegendLabel,
      zAxisIncompleteConnectionHint:
        "Configure stop words on this workstation before connecting Continue or Reject routes.",
    },
    "zh-CN": {
      addDialogAddEntityAction: "添加实体",
      addDialogAssignedWorkerLabel: "分配的工作者",
      addDialogAssignedWorkerPlaceholder: "选择一个工作者",
      addDialogCancelAction: "取消",
      addDialogCapacityLabel: "容量",
      addDialogDocContentHelp: "保存到捆绑文档条目的可选 UTF-8 文本。",
      addDialogDocContentLabel: "文档文本",
      addDialogDocFileNameHelp: "保存到 factory/docs/ 下。请包含文件扩展名。",
      addDialogDocFileNameLabel: "文件名",
      addDialogDescription: (kind) => {
        if (kind === "doc") {
          return "在 factory/docs 下创建可在保存前编辑文本的捆绑文档。";
        }
        if (kind === "workstation") {
          return "在当前图草稿中创建一个待处理工作站。";
        }
        if (kind === "work-type") {
          return "定义一个新的工作类型及其首个有序状态。";
        }
        if (kind === "work-state") {
          return "向现有工作类型追加一个新的有序状态。";
        }
        if (kind === "worker") {
          return "在当前图草稿中创建一个待处理的模型或脚本工作者。";
        }
        return `在当前图草稿中创建一个待处理的${kind}。`;
      },
      addDialogFirstStateHelp: "新的工作类型必须从一个有序状态开始。",
      addDialogFirstStateLabel: "首个状态",
      addDialogIdentifierHelp: "使用应保存到工厂定义中的已编写名称。",
      addDialogIdentifierLabel: "标识符",
      addDialogKindLabel: "类型",
      addDialogModelHelp: "将保存到新推理 worker 上的模型标识符。",
      addDialogModelLabel: "模型",
      addDialogModelOperationAddAction: "添加操作",
      addDialogModelOperationHeading: (operationIndex) =>
        `操作 ${operationIndex + 1}`,
      addDialogModelOperationInputsLabel: "输入槽位",
      addDialogModelOperationNameHelp: "请使用大写名称，例如 TTS 或 ASR。",
      addDialogModelOperationNameLabel: "操作名称",
      addDialogModelOperationOutputsLabel: "输出槽位",
      addDialogModelOperationRemoveAction: "移除操作",
      addDialogModelOperationSlotAddAction: (direction) =>
        direction === "input" ? "添加输入槽位" : "添加输出槽位",
      addDialogModelOperationSlotContentTypesLabel: "内容类型",
      addDialogModelOperationSlotHeading: (direction, slotIndex) =>
        `${direction === "input" ? "输入" : "输出"}槽位 ${slotIndex + 1}`,
      addDialogModelOperationSlotNameLabel: "槽位名称",
      addDialogModelOperationSlotRemoveAction: "移除槽位",
      addDialogModelOperationSlotRequiredLabel: "必填输入槽位",
      addDialogModelOperationsHelp:
        "可选择为推理运行工作站声明带有类型化输入和输出槽位的提供方无关操作。",
      addDialogModelOperationsLabel: "模型操作（可选）",
      addDialogPromptBodyEditorError:
        "提示词编辑器无法启动。请先在下方编辑提示正文，我们稍后会恢复编辑器。",
      addDialogPromptBodyEditorLoading: "正在启动提示词编辑器。",
      addDialogPromptBodyHelp: "工作站正文的可选提示内容。",
      addDialogPromptBodyLabel: "提示正文",
      addDialogStateTypeLabel: "状态类型",
      addDialogTitle: (kind) => {
        if (kind === "doc") {
          return "添加文档";
        }
        if (kind === "work-type") {
          return "添加工作类型";
        }
        if (kind === "work-state") {
          return "添加工作状态";
        }
        return `添加${kind}`;
      },
      addDialogWorkTypeLabel: "工作类型",
      addDialogWorkTypePlaceholder: "选择一个工作类型",
      addMenuAction: (kind) => {
        switch (kind) {
          case "doc":
            return {
              description: "在 factory/docs 下添加可编辑文本的捆绑文档文件。",
              label: "文档",
            };
          case "workstation":
            return {
              description: "创建一个待处理工作站并分配现有工作者。",
              label: "工作站",
            };
          case "worker":
            return {
              description: "添加可分配给工作站的模型或脚本工作者。",
              label: "工作者",
            };
          case "work-type":
            return {
              description: "定义一个包含首个有序状态的新工作类型。",
              label: "工作类型",
            };
          case "work-state":
            return {
              description: "向现有工作类型追加一个状态。",
              label: "工作状态",
            };
          case "resource":
            return {
              description: "注册工作者或工作站可消耗的资源。",
              label: "资源",
            };
        }
      },
      connectionAnchorDescription: (anchorId) => {
        switch (anchorId) {
          case "worker-resource-source":
            return "将此资源提供给工作者。";
          case "workstation-resource-source":
            return "将此资源提供给工作站。";
          case "worker-input-target":
            return "接收此工作者所需的资源。";
          case "worker-assignment-source":
            return "将此工作者分配给工作站。";
          case "workstation-input-source":
            return "将此工作状态路由到工作站输入。";
          case "workstation-input-target":
            return "接收此工作站的输入工作状态。";
          case "work-state-input-target":
            return "接收工作站输出到此工作状态。";
          case "worker-assignment-target":
            return "接收此工作站的工作者分配。";
          case "workstation-resource-target":
            return "接收此工作站的资源需求。";
          case "workstation-output-source":
            return "从此工作站路由成功输出。";
          case "workstation-on-continue-source":
            return "从此工作站路由继续转换。";
          case "workstation-on-failure-source":
            return "从此工作站路由失败转换。";
          case "workstation-on-rejection-source":
            return "从此工作站路由拒绝转换。";
          default:
            return "";
        }
      },
      connectionAnchorLabel: (anchorId) => {
        switch (anchorId) {
          case "worker-resource-source":
          case "worker-assignment-target":
            return "工作者";
          case "workstation-resource-source":
            return "工作站";
          case "worker-input-target":
          case "workstation-resource-target":
            return "资源";
          case "worker-assignment-source":
            return "分配";
          case "workstation-input-source":
          case "workstation-input-target":
            return "输入";
          case "workstation-output-source":
            return "成功";
          case "workstation-on-continue-source":
            return "继续";
          case "workstation-on-failure-source":
            return "失败";
          case "workstation-on-rejection-source":
            return "拒绝";
          case "work-state-input-target":
            return "输入";
          default:
            return "";
        }
      },
      connectionFallbackNotice: "请先选择兼容的源锚点和目标锚点再创建连接。",
      connectionIncompatibleNotice: (
        sourceAnchor,
        sourceNode,
        targetAnchor,
        targetNode,
      ) =>
        `${sourceNode} 的${sourceAnchor}连接不能连接到 ${targetNode} 上的${targetAnchor}。`,
      connectionSelectSourceNotice: "请先选择源锚点，再选择目标锚点。",
      defaultWorkTypeLabel: "默认工作类型",
      draftActionsAriaLabel: "待处理图更改",
      draftActionsDiscard: "放弃更改",
      draftActionsSave: "保存更改",
      draftActionsSaving: "保存中...",
      draftActionsTitle: "待处理图更改",
      edgeAriaLabel: (label, source, target) =>
        `${label}：从 ${source} 到 ${target}`,
      edgeWaypointAddLabel: "添加路径点",
      edgeWaypointHandleLabel: (index) => `移动边路径点 ${index + 1}`,
      edgeWaypointRemoveLabel: (index) => `移除路径点 ${index + 1}`,
      edgeWaypointKindLabel: "类型",
      edgeWaypointSelectedLabel: "已选边路由",
      edgeWaypointSourceLabel: "来源",
      edgeWaypointTargetLabel: "目标",
      visualGroupAriaLabel: (group) =>
        group.label?.trim()
          ? `视觉分组 ${group.label.trim()}`
          : `视觉分组 ${group.id}`,
      visualGroupColorLabel: "分组颜色",
      visualGroupColorOptionLabel: (token) => `使用 ${token} 分组颜色`,
      visualGroupEmptyLabelError: "请输入分组标签。",
      visualGroupInvalidBoundsError:
        "分组边界包含非有限几何。请在保存前调整分组大小以修正。",
      visualGroupLabelFieldLabel: "分组标签",
      visualGroupMembershipEmptyLabel: "当前画布上没有可分配的节点。",
      visualGroupMembershipLabel: "分组成员",
      visualGroupMembershipNodeLabel: (label) => `将 ${label} 加入此分组`,
      visualGroupMembershipStaleNodeLabel: (nodeId) =>
        `已保存的成员 ${nodeId} 已不在画布上。`,
      visualGroupSelectedLabel: "已选视觉分组",
      visualGroupDeleteLabel: "删除分组",
      visualGroupResizeHandleLabel: (corner) => `从 ${corner} 角调整分组大小`,
      toolbarCreateGroupDescription: "创建带标签的背景分组",
      toolbarCreateGroupLabel: "创建分组",
      edgeKindLabel: (kind) => {
        switch (kind) {
          case "worker-assignment":
            return "工作者分配";
          case "worker-resource":
            return "工作者资源";
          case "work-type-state":
            return "状态归属";
          case "workstation-input":
            return "输入路由";
          case "workstation-on-continue":
            return "继续路由";
          case "workstation-on-failure":
            return "失败路由";
          case "workstation-on-rejection":
            return "拒绝路由";
          case "workstation-output":
            return "成功路由";
          case "workstation-resource":
            return "工作站资源";
        }
      },
      flowConnectionHint: "请使用带标签的锚点创建兼容连接。",
      flowPendingLabel: "待处理",
      flowRemovingLabel: "移除中",
      kindLabel: (kind) => {
        switch (kind) {
          case "doc":
            return "文档";
          case "resource":
            return "资源";
          case "worker":
            return "工作者";
          case "workstation":
            return "工作站";
          case "work-type":
            return "工作类型";
          case "work-state":
            return "工作状态";
        }
      },
      leaveDialogBody:
        "保存会保留待处理的工厂拓扑，放弃会恢复到最新的服务器图，也可以继续编辑。",
      leaveDialogDescription: "此图编辑会话仍有本地拓扑更改。",
      leaveDialogKeepEditing: "继续编辑",
      leaveDialogTitle: "带着未保存的更改离开图编辑器？",
      modeActive: "编辑器模式已启用",
      modeClassifierRoutesUnavailable: (workstationName) =>
        `工厂图编辑器暂不支持分类工作站路由。“${workstationName}”在此视图中将保持只读，直到支持带标签的路由编辑。`,
      modeEnterEditor: "编辑模式",
      modeLeaveEditor: "离开编辑器",
      modeLoadingDefinition: "正在加载编辑器定义",
      modeUnavailablePrefix: "编辑器不可用",
      modeUnsavedChanges: "未保存的更改",
      modeObserve: "观察",
      noticeConnectionBlockedTitle: "连接被阻止",
      noticeEmptyMessage: "该工厂尚未发布任何工作站图。",
      noticeEmptyTitle: "尚未加载工作流拓扑",
      noticeDismissLabel: "关闭",
      noticePanelCollapseLabel: "折叠编辑器提醒",
      noticePanelCount: (count) => `${count} 个问题`,
      noticePanelExpandLabel: "展开编辑器提醒",
      noticePanelTitle: "编辑器提醒",
      noticeRemovalBlockedTitle: "移除被阻止",
      noticeSaveFailedAffectedSummary: (labels) => `受影响项：${labels}`,
      noticeSaveFailedTitle: "拓扑保存失败",
      noticeSaveSuccessDescription:
        "草稿已清除，图正在等待最新的 factory-change 事件刷新。",
      noticeSaveSuccessTitle: "拓扑已保存",
      noticeStaleDescription:
        "请在保存前刷新或放弃当前草稿，以免覆盖较新的拓扑版本。",
      noticeStaleTitle: "有更新的工厂定义可用",
      noticeTopologyBlockedDescription:
        "当前工厂仍有活动工作在运行，因此无法保存。",
      noticeTopologyBlockedTitle: "拓扑编辑被阻止",
      noticeLayoutWarningTitle: "可恢复的布局警告",
      noticeValidationFailureTitle: "工厂验证问题",
      operationConnectionInvalid: "图连接无效。",
      operationEdgeNotFound: (edgeId) => `未找到图边“${edgeId}”。`,
      operationGraphEditsInvalid: "图编辑必须有效后才能应用。",
      operationNodeNotFound: (nodeId) => `未找到图节点“${nodeId}”。`,
      dirtyStateSummary: (dirtyState) => {
        if (
          dirtyState.preferencesDirty &&
          !dirtyState.layoutDirty &&
          !dirtyState.topologyDirty
        ) {
          return "私有视图偏好已更改";
        }
        if (dirtyState.layoutDirty && dirtyState.topologyDirty) {
          return "未保存的布局和拓扑更改";
        }
        if (dirtyState.layoutDirty) {
          return "未保存的布局更改";
        }
        if (dirtyState.topologyDirty) {
          return "未保存的拓扑更改";
        }
        return "未保存的更改";
      },
      saveConfirmAction: (kind) => {
        switch (kind) {
          case "layout-only":
            return "保存布局";
          case "topology-only":
            return "保存拓扑";
          case "mixed":
            return "保存更改";
          case "preferences-only":
            return "保存偏好";
          case "none":
            return "保存更改";
        }
      },
      saveBlockedActiveWork: "此工厂仍有活动工作在运行，因此无法保存拓扑。",
      saveBlockedStaleDraft:
        "此草稿打开后收到了更新的工厂拓扑。请先刷新或放弃再保存。",
      saveConfirmTitle: "保存工厂图更改？",
      saveSummaryDescription: (summary) => {
        const segments = [
          summary.createdEntities > 0
            ? `${summary.createdEntities} 个新增实体`
            : null,
          summary.removedEntities > 0
            ? `${summary.removedEntities} 个删除实体`
            : null,
          summary.changedEdges > 0 ? `${summary.changedEdges} 条更改边` : null,
        ].filter((segment) => segment !== null);

        if (segments.length === 0) {
          return "没有待处理的图拓扑更改。";
        }
        if (segments.length === 1) {
          return `此保存将应用 ${segments[0]}。`;
        }
        const finalSegment = segments[segments.length - 1];
        return `此保存将应用 ${segments.slice(0, -1).join("、")} 和 ${finalSegment}。`;
      },
      saveSummaryForDirtyState: (summary) => {
        switch (summary.kind) {
          case "preferences-only":
            return "可见性和筛选偏好仅针对你的视图更改。它们会保持私有，不会保存到共享工厂文档中。";
          case "layout-only":
            return "此保存将更新共享图布局、视觉分组和视口。工厂拓扑保持不变。";
          case "topology-only":
            return summary.topologySummary;
          case "mixed":
            return `此保存将更新共享图布局并应用拓扑更改：${summary.topologySummary.replace(/^此保存将应用 /, "").replace(/\.$/, "")}。`;
          case "none":
            return "没有待处理的共享工厂文档更改。";
        }
      },
      stateCollapsed: "已折叠",
      stateTypeLabel: (stateType) => {
        switch (stateType) {
          case "INITIAL":
            return "初始";
          case "PROCESSING":
            return "处理中";
          case "TERMINAL":
            return "终止";
          case "FAILED":
            return "失败";
        }
      },
      stateVisible: "可见",
      toolbarAddDescription: "添加图实体",
      toolbarAddLabel: "添加",
      toolbarAriaLabel: "工厂图编辑器工具",
      toolbarConnectDescription: "在图上连接节点",
      toolbarConnectLabel: "连接",
      toolbarDeleteDescription: "从图中删除节点或边",
      toolbarDeleteDisabledNoSelectionDescription: "选择要删除的图项",
      toolbarDeleteDisabledNoSelectionLabel: "删除，未选择图项",
      toolbarDeleteDisabledNonDeletableDescription: "所选图项无法删除",
      toolbarDeleteDisabledNonDeletableLabel: "删除，所选图项无法删除",
      toolbarDeleteLabel: "删除",
      toolbarDeleteMultiSelectionDescription: (count) => `删除 ${count} 个所选图项`,
      toolbarDeleteMultiSelectionLabel: (count) => `删除 ${count} 个所选图项`,
      toolbarDeleteSelectionDescription: "删除所选图项",
      toolbarDeleteSingleSelectionDescription: "删除所选图项",
      toolbarDeleteSingleSelectionLabel: "删除所选图项",
      toolbarRedoDescription: "重做上一条已撤销的布局更改",
      toolbarRedoLabel: "重做",
      toolbarResetLayoutDescription: "将节点位置重置为已保存的共享布局基线",
      toolbarResetLayoutLabel: "重置布局",
      toolbarUndoDescription: "撤销上一条布局更改",
      toolbarUndoLabel: "撤销",
      toolbarHideShowDescription: "在图上显示或隐藏节点类别",
      toolbarHideShowLabel: "显示或隐藏",
      toolbarHideShowMenuAriaLabel: "工厂图节点类别可见性菜单",
      toolbarHideShowMenuDescription:
        "切换哪些节点类别显示在图上。隐藏的类别会保持不可见，直到你再次显示它们。",
      toolbarHideShowMenuTitle: "隐藏或显示节点类别",
      toolbarClearPreferencesDescription:
        "恢复此工厂的共享创作图视图。私有的可见性和筛选选择将被清除。",
      toolbarClearPreferencesLabel: "清除私有视图偏好",
      toolbarOpenAddMenuLabel: "添加",
      toolbarOpenHideShowMenuLabel: "显示或隐藏",
      nodeClassVisibilityDescription: (kind) => {
        const label = getFactoryGraphEditorMessages("zh-CN").kindLabel(kind);
        return `在图上显示${label}节点。`;
      },
      toolbarVisibilityMenuAriaLabel: "添加图实体菜单",
      toolbarVisibilityMenuDescription: "选择要添加到当前草稿的受支持实体。",
      toolbarVisibilityMenuTitle: "添加图实体",
      visibilityPresetAllLabel: "全部",
      visibilityPresetExecutionLabel: "执行",
      visibilityPresetInfrastructureLabel: "基础设施",
      visibilityPresetWorkflowLabel: "工作流",
      visibilityPresetsAriaLabel: "工厂图可见性预设",
      viewportLabel: "工作图视口",
      validationDuplicateIdentifier: (nodeId) =>
        `不允许重复的图标识符“${nodeId}”。`,
      validationIncompatibleEdge: (relationship, source, target) =>
        `关系“${relationship}”不能将 ${source} 连接到 ${target}。`,
      validationMissingRequiredIdentifier: (kind) =>
        `保存前必须填写${getFactoryGraphEditorMessages("zh-CN").kindLabel(kind)}标识符。`,
      validationMissingWorkerAssignment: (workstationName) =>
        `工作站“${workstationName}”必须保留一个工作者分配。`,
      validationUnknownWorkstationRoute: ({
        routeField,
        stateName,
        workstationName,
        workTypeName,
      }) =>
        `工作站“${workstationName}”的 ${routeField} 引用了未知工作状态“${workTypeName}:${stateName}”。`,
      validationUnknownEdgeNode: (relationship, which) =>
        `关系“${relationship}”引用了未知的${which === "source" ? "源" : "目标"}节点。`,
      removalDescription: ({
        connectedEdgeCount,
        impactedStateCount,
        kind,
        label,
      }) => {
        const edgeSummary =
          connectedEdgeCount > 0
            ? `这将移除 ${connectedEdgeCount} 条图边。`
            : "此实体没有要移除的已连接图边。";

        if (kind === "work-type") {
          return `${edgeSummary} ${label} 还拥有 ${impactedStateCount} 个工作状态，这些状态也会一并移除。`;
        }
        if (kind === "work-state") {
          return `${edgeSummary} 引用 ${label} 的所有工作站路由都将从待处理草稿中清除。`;
        }
        if (kind === "resource") {
          return `${edgeSummary} 依赖 ${label} 的工作者和工作站资源引用都将从待处理草稿中清除。`;
        }
        return edgeSummary;
      },
      removalEdgeConfirmLabel: (edgeLabel) => `移除${edgeLabel}`,
      removalEdgeDescription: (kind, source, target) => {
        switch (kind) {
          case "worker-assignment":
            return `这会将 ${source} 从 ${target} 取消分配。该工作站需要另一个工作者后拓扑保存才能成功。`;
          case "worker-resource":
            return `这会从待处理草稿中移除 ${target} 的可用资源 ${source}。`;
          case "workstation-resource":
            return `这会从待处理草稿中移除 ${target} 的必需资源 ${source}。`;
          case "workstation-input":
            return `这会停止将 ${source} 路由到 ${target}。`;
          case "workstation-output":
            return `这会移除从 ${source} 到 ${target} 的成功路由。`;
          case "workstation-on-continue":
            return `这会移除从 ${source} 到 ${target} 的继续路由。`;
          case "workstation-on-failure":
            return `这会移除从 ${source} 到 ${target} 的失败路由。`;
          case "workstation-on-rejection":
            return `这会移除从 ${source} 到 ${target} 的拒绝路由。`;
          case "work-type-state":
            return "";
        }
      },
      removalEdgeIneligibleWorkTypeState:
        "工作类型排序边由工作状态归属管理，不能直接移除。",
      removalEdgeLabel: (kind, source) => {
        switch (kind) {
          case "worker-assignment":
            return `${source} 分配`;
          case "worker-resource":
          case "workstation-resource":
            return `${source} 资源链接`;
          case "workstation-input":
            return `${source} 输入路由`;
          case "workstation-output":
            return `${source} 成功路由`;
          case "workstation-on-continue":
            return `${source} 继续路由`;
          case "workstation-on-failure":
            return `${source} 失败路由`;
          case "workstation-on-rejection":
            return `${source} 拒绝路由`;
          case "work-type-state":
            return `${source} 状态归属`;
        }
      },
      removalEdgeTitle: (edgeLabel) => `移除${edgeLabel}？`,
      removalEntityConfirmLabel: (label, kind) =>
        `删除 ${label} ${getFactoryGraphEditorMessages("zh-CN").kindLabel(kind)}`,
      removalEntityTitle: (label, kind) =>
        `移除 ${label} ${getFactoryGraphEditorMessages("zh-CN").kindLabel(kind)}？`,
      removalDocConfirmLabel: (displayLabel) => `删除 ${displayLabel} 文档`,
      removalDocDescription: (targetPath) =>
        `这将从当前工厂草稿中移除位于 ${targetPath} 的捆绑文档。`,
      removalDocTitle: (displayLabel) => `移除 ${displayLabel} 文档？`,
      removalFallbackConfirmDescription: "从当前草稿中移除此图实体。",
      removalFallbackConfirmLabel: "删除实体",
      removalBatchConfirmLabel: (itemCount) => `删除 ${itemCount} 个所选图项`,
      removalBatchDescription: (itemCount) =>
        `这将从当前草稿中移除 ${itemCount} 个所选图项。`,
      removalBatchTitle: (itemCount) => `移除 ${itemCount} 个所选图项？`,
      removalFallbackTitle: "移除图实体？",
      removalWorkerAssignedReason: (workstationCount, workerLabel) =>
        `此工作者仍分配给 ${workstationCount} 个工作站。删除 ${workerLabel} 前，请重新分配或移除这些工作站。`,
      localizeModelOperationContentType: (contentType) => {
        switch (contentType) {
          case "TEXT":
            return "文本";
          case "IMAGE":
            return "图像";
          case "AUDIO":
            return "音频";
          case "JSON":
            return "JSON";
          case "BINARY":
            return "二进制";
        }
      },
      workerStatusLabel: (status) => {
        switch (status) {
          case "active":
            return "活跃";
          case "errored":
            return "错误";
          case "idle":
            return "空闲";
          case "unavailable":
            return "不可用";
        }
      },
      workStatePhaseLegendAriaLabel: "工作状态生命周期颜色",
      workStatePhaseLegendLabel: (stateType) => {
        switch (stateType) {
          case "INITIAL":
            return "初始";
          case "PROCESSING":
            return "处理中";
          case "TERMINAL":
            return "已完成";
          case "FAILED":
            return "失败";
        }
      },
      zAxisIncompleteConnectionHint:
        "请在此工作站配置停止词后，再连接“继续”或“拒绝”路由。",
    },
  };

export function getFactoryGraphEditorMessages(
  locale?: string | null,
): FactoryGraphEditorMessages {
  return resolveLocalizedMessages(factoryGraphEditorMessagesByLocale, locale);
}

export { factoryGraphEditorMessagesByLocale };
