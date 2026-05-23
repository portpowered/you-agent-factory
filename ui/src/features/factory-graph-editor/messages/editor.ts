import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import type { FactoryGraphNodeKind } from "../lib/factory-graph-draft-types";
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
  saveConfirmAction: string;
  saveConfirmTitle: string;
  stateCollapsed: string;
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
      draftActionsAriaLabel: "Pending graph changes",
      draftActionsDiscard: "Discard changes",
      draftActionsSave: "Save changes",
      draftActionsSaving: "Saving...",
      draftActionsTitle: "Pending graph changes",
      edgeAriaLabel: (label, source, target) => `${label} from ${source} to ${target}`,
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
      saveConfirmAction: "Save topology",
      saveConfirmTitle: "Save factory graph changes?",
      stateCollapsed: "Collapsed",
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
      draftActionsAriaLabel: "待处理图更改",
      draftActionsDiscard: "放弃更改",
      draftActionsSave: "保存更改",
      draftActionsSaving: "保存中...",
      draftActionsTitle: "待处理图更改",
      edgeAriaLabel: (label, source, target) => `${label}：从 ${source} 到 ${target}`,
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
      saveConfirmAction: "保存拓扑",
      saveConfirmTitle: "保存工厂图更改？",
      stateCollapsed: "已折叠",
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
