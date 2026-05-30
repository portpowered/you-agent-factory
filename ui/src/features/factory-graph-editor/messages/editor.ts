// biome-ignore lint/nursery/noExcessiveLinesPerFile: factory graph editor copy stays consolidated so message keys and locale fallbacks remain auditable during hardcoded-copy cleanup.
import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type {
  FactoryGraphNodeKind,
  FactoryWorkState,
} from "../lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import type { FactoryGraphWorkerRuntimeStatus } from "../lib/factory-graph-editor-runtime";

export interface FactoryGraphEditorMessages {
  addDialogAddEntityAction: string;
  addDialogAssignedWorkerLabel: string;
  addDialogAssignedWorkerPlaceholder: string;
  addDialogCancelAction: string;
  addDialogCapacityLabel: string;
  addDialogDescription: (kind: FactoryGraphAddEntityDraft["kind"]) => string;
  addDialogFirstStateHelp: string;
  addDialogFirstStateLabel: string;
  addDialogIdentifierHelp: string;
  addDialogIdentifierLabel: string;
  addDialogKindLabel: string;
  addDialogModelHelp: string;
  addDialogModelLabel: string;
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
  draftActionsAriaLabel: string;
  draftActionsDiscard: string;
  draftActionsSave: string;
  draftActionsSaving: string;
  draftActionsTitle: string;
  edgeAriaLabel: (label: string, source: string, target: string) => string;
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
  modeNoDraftChanges: string;
  modePendingChanges: string;
  modeUnavailablePrefix: string;
  modeUnsavedChanges: string;
  modeObserve: string;
  noticeConnectionBlockedTitle: string;
  noticeEmptyMessage: string;
  noticeEmptyTitle: string;
  noticeRemovalBlockedTitle: string;
  noticeSaveFailedTitle: string;
  noticeSaveSuccessDescription: string;
  noticeSaveSuccessTitle: string;
  noticeStaleDescription: string;
  noticeStaleTitle: string;
  noticeTopologyBlockedDescription: string;
  noticeTopologyBlockedTitle: string;
  noticeValidationFailureTitle: string;
  operationConnectionInvalid: string;
  operationEdgeNotFound: (edgeId: string) => string;
  operationGraphEditsInvalid: string;
  operationNodeNotFound: (nodeId: string) => string;
  saveConfirmAction: string;
  saveBlockedActiveWork: string;
  saveBlockedStaleDraft: string;
  saveConfirmTitle: string;
  saveSummaryDescription: (summary: {
    changedEdges: number;
    createdEntities: number;
    removedEntities: number;
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
  toolbarDeleteLabel: string;
  toolbarOpenAddMenuLabel: string;
  toolbarPendingChanges: string;
  toolbarNoPendingChanges: string;
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
  removalFallbackConfirmDescription: string;
  removalFallbackConfirmLabel: string;
  removalFallbackTitle: string;
  removalWorkerAssignedReason: (
    workstationCount: number,
    workerLabel: string,
  ) => string;
  workerStatusLabel: (status: FactoryGraphWorkerRuntimeStatus) => string;
}

function describeEnglishAddDialog(kind: FactoryGraphAddEntityDraft["kind"]) {
  if (kind === "workstation") {
    return "Create a pending workstation in the current graph draft.";
  }
  if (kind === "work-type") {
    return "Define a new work type and its first ordered state.";
  }
  if (kind === "work-state") {
    return "Append a new ordered state to an existing work type.";
  }
  return `Create a pending ${kind} in the current graph draft.`;
}

function describeEnglishAddDialogTitle(
  kind: FactoryGraphAddEntityDraft["kind"],
) {
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
    return "No graph changes are pending.";
  }
  if (segments.length === 1) {
    return `This save will apply ${segments[0]}.`;
  }

  const finalSegment = segments[segments.length - 1];
  return `This save will apply ${segments.slice(0, -1).join(", ")} and ${finalSegment}.`;
}

function describeEnglishAddMenuAction(
  kind: FactoryGraphAddEntityDraft["kind"],
) {
  switch (kind) {
    case "workstation":
      return {
        description:
          "Create a pending workstation and assign an existing worker.",
        label: "Workstation",
      };
    case "worker":
      return {
        description: "Add a model worker that can be assigned to workstations.",
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

const factoryGraphEditorMessagesByLocale: LocalizedMessageCatalog<FactoryGraphEditorMessages> =
  {
    en: {
      addDialogAddEntityAction: "Add entity",
      addDialogAssignedWorkerLabel: "Assigned worker",
      addDialogAssignedWorkerPlaceholder: "Select a worker",
      addDialogCancelAction: "Cancel",
      addDialogCapacityLabel: "Capacity",
      addDialogDescription: describeEnglishAddDialog,
      addDialogFirstStateHelp:
        "New work types start with one required ordered state.",
      addDialogFirstStateLabel: "First state",
      addDialogIdentifierHelp:
        "Use the authored name the factory definition should save.",
      addDialogIdentifierLabel: "Identifier",
      addDialogKindLabel: "Kind",
      addDialogModelHelp:
        "The model identifier saved on the new `MODEL_WORKER`.",
      addDialogModelLabel: "Model",
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
      draftActionsAriaLabel: "Pending graph changes",
      draftActionsDiscard: "Discard changes",
      draftActionsSave: "Save changes",
      draftActionsSaving: "Saving...",
      draftActionsTitle: "Pending graph changes",
      edgeAriaLabel: (label, source, target) =>
        `${label} from ${source} to ${target}`,
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
      modeEnterEditor: "Enter factory graph editor",
      modeLeaveEditor: "Leave factory graph editor",
      modeLoadingDefinition: "Loading editor definition",
      modeNoDraftChanges: "No draft changes",
      modePendingChanges: "Draft changes pending",
      modeUnavailablePrefix: "Editor unavailable",
      modeUnsavedChanges: "Unsaved graph changes",
      modeObserve: "Observe mode",
      noticeConnectionBlockedTitle: "Connection blocked",
      noticeEmptyMessage:
        "The factory has not published any workstation graph yet.",
      noticeEmptyTitle: "No workflow topology loaded",
      noticeRemovalBlockedTitle: "Removal blocked",
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
      noticeValidationFailureTitle: "Factory validation issue",
      operationConnectionInvalid: "Graph connection is invalid.",
      operationEdgeNotFound: (edgeId) =>
        `Graph edge "${edgeId}" was not found.`,
      operationGraphEditsInvalid:
        "Graph edits must be valid before they can be applied.",
      operationNodeNotFound: (nodeId) =>
        `Graph node "${nodeId}" was not found.`,
      saveConfirmAction: "Save topology",
      saveBlockedActiveWork:
        "Topology save is unavailable while active work is still running in this factory.",
      saveBlockedStaleDraft:
        "A newer factory topology arrived while this draft was open. Refresh or discard before saving.",
      saveConfirmTitle: "Save factory graph changes?",
      saveSummaryDescription: describeEnglishSaveSummary,
      stateCollapsed: "Collapsed",
      stateTypeLabel: describeEnglishStateType,
      stateVisible: "Visible",
      toolbarAddDescription: "Add supported graph entities",
      toolbarAddLabel: "Add",
      toolbarAriaLabel: "Factory graph editor tools",
      toolbarConnectDescription: "Create compatible graph connections",
      toolbarConnectLabel: "Connect",
      toolbarDeleteDescription: "Remove nodes or edges from the draft",
      toolbarDeleteLabel: "Delete",
      toolbarOpenAddMenuLabel: "Open add entity menu",
      toolbarPendingChanges: "Draft changes pending",
      toolbarNoPendingChanges: "No draft changes",
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
      removalFallbackConfirmDescription:
        "Remove this graph entity from the current draft.",
      removalFallbackConfirmLabel: "Delete entity",
      removalFallbackTitle: "Remove graph entity?",
      removalWorkerAssignedReason: (workstationCount, workerLabel) =>
        `This worker is still assigned to ${pluralizeEnglish(
          workstationCount,
          "workstation",
        )}. Reassign or remove those workstations before deleting ${workerLabel}.`,
      workerStatusLabel: describeEnglishWorkerStatus,
    },
    "zh-CN": {
      addDialogAddEntityAction: "添加实体",
      addDialogAssignedWorkerLabel: "分配的工作者",
      addDialogAssignedWorkerPlaceholder: "选择一个工作者",
      addDialogCancelAction: "取消",
      addDialogCapacityLabel: "容量",
      addDialogDescription: (kind) => {
        if (kind === "workstation") {
          return "在当前图草稿中创建一个待处理工作站。";
        }
        if (kind === "work-type") {
          return "定义一个新的工作类型及其首个有序状态。";
        }
        if (kind === "work-state") {
          return "向现有工作类型追加一个新的有序状态。";
        }
        return `在当前图草稿中创建一个待处理的${kind}。`;
      },
      addDialogFirstStateHelp: "新的工作类型必须从一个有序状态开始。",
      addDialogFirstStateLabel: "首个状态",
      addDialogIdentifierHelp: "使用应保存到工厂定义中的已编写名称。",
      addDialogIdentifierLabel: "标识符",
      addDialogKindLabel: "类型",
      addDialogModelHelp: "将保存到新 `MODEL_WORKER` 上的模型标识符。",
      addDialogModelLabel: "模型",
      addDialogPromptBodyHelp: "工作站正文的可选提示内容。",
      addDialogPromptBodyLabel: "提示正文",
      addDialogStateTypeLabel: "状态类型",
      addDialogTitle: (kind) => {
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
          case "workstation":
            return {
              description: "创建一个待处理工作站并分配现有工作者。",
              label: "工作站",
            };
          case "worker":
            return {
              description: "添加可分配给工作站的模型工作者。",
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
      draftActionsAriaLabel: "待处理图更改",
      draftActionsDiscard: "放弃更改",
      draftActionsSave: "保存更改",
      draftActionsSaving: "保存中...",
      draftActionsTitle: "待处理图更改",
      edgeAriaLabel: (label, source, target) =>
        `${label}：从 ${source} 到 ${target}`,
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
      modeEnterEditor: "进入工厂图编辑器",
      modeLeaveEditor: "离开工厂图编辑器",
      modeLoadingDefinition: "正在加载编辑器定义",
      modeNoDraftChanges: "没有草稿更改",
      modePendingChanges: "草稿更改待处理",
      modeUnavailablePrefix: "编辑器不可用",
      modeUnsavedChanges: "存在未保存的图更改",
      modeObserve: "观察模式",
      noticeConnectionBlockedTitle: "连接被阻止",
      noticeEmptyMessage: "该工厂尚未发布任何工作站图。",
      noticeEmptyTitle: "尚未加载工作流拓扑",
      noticeRemovalBlockedTitle: "移除被阻止",
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
      noticeValidationFailureTitle: "工厂验证问题",
      operationConnectionInvalid: "图连接无效。",
      operationEdgeNotFound: (edgeId) => `未找到图边“${edgeId}”。`,
      operationGraphEditsInvalid: "图编辑必须有效后才能应用。",
      operationNodeNotFound: (nodeId) => `未找到图节点“${nodeId}”。`,
      saveConfirmAction: "保存拓扑",
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
          return "没有待处理的图更改。";
        }
        if (segments.length === 1) {
          return `此保存将应用 ${segments[0]}。`;
        }
        const finalSegment = segments[segments.length - 1];
        return `此保存将应用 ${segments.slice(0, -1).join("、")} 和 ${finalSegment}。`;
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
      toolbarAddDescription: "添加受支持的图实体",
      toolbarAddLabel: "添加",
      toolbarAriaLabel: "工厂图编辑器工具",
      toolbarConnectDescription: "创建兼容的图连接",
      toolbarConnectLabel: "连接",
      toolbarDeleteDescription: "从草稿中移除节点或边",
      toolbarDeleteLabel: "删除",
      toolbarOpenAddMenuLabel: "打开添加实体菜单",
      toolbarPendingChanges: "草稿更改待处理",
      toolbarNoPendingChanges: "没有草稿更改",
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
      removalFallbackConfirmDescription: "从当前草稿中移除此图实体。",
      removalFallbackConfirmLabel: "删除实体",
      removalFallbackTitle: "移除图实体？",
      removalWorkerAssignedReason: (workstationCount, workerLabel) =>
        `此工作者仍分配给 ${workstationCount} 个工作站。删除 ${workerLabel} 前，请重新分配或移除这些工作站。`,
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
    },
  };

export function getFactoryGraphEditorMessages(
  locale?: string | null,
): FactoryGraphEditorMessages {
  return resolveLocalizedMessages(factoryGraphEditorMessagesByLocale, locale);
}

export { factoryGraphEditorMessagesByLocale };
