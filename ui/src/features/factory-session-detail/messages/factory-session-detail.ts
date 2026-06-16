import type { components } from "../../../api/generated/openapi";
import {
  FactoryOrchestratorKind,
  FactorySessionDurableLifecycleStatus,
  FactorySessionJavaScriptScriptStatus,
  FactorySessionStatus,
} from "../../../api/generated/openapi";
import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactorySessionDetailMessages {
  artifactsHeading: string;
  checkpointRefsHeading: string;
  childDispatchCountsLabel: string;
  dispatchesHeading: string;
  durableLifecycleStatusLabels: Record<
    components["schemas"]["FactorySessionDurableLifecycleStatus"],
    string
  >;
  dynamicWorkflowShorthand: string;
  enabledTransitionsHeading: string;
  errorState: string;
  finalResultRefLabel: string;
  javascriptProjectionMissingState: string;
  loadingState: string;
  markingEmptyState: string;
  markingHeading: string;
  missingState: string;
  orchestratorKindLabel: string;
  orchestratorKindLabels: Record<
    components["schemas"]["FactoryOrchestratorKind"],
    string
  >;
  partialResultRefLabel: string;
  phaseLabel: string;
  phasesLabel: string;
  runtimeHeading: string;
  runtimeStatusLabels: Record<
    components["schemas"]["FactorySessionStatus"],
    string
  >;
  scriptStatusLabel: string;
  scriptStatusLabels: Record<
    components["schemas"]["FactorySessionJavaScriptScriptStatus"],
    string
  >;
  selectedSessionHeading: string;
  sessionIdLabel: string;
  statusLabel: string;
  warningsHeading: string;
}

const englishRuntimeStatusLabels = {
  [FactorySessionStatus.ACTIVE]: "Active",
  [FactorySessionStatus.FINISHED]: "Finished",
  [FactorySessionStatus.IDLE]: "Idle",
} satisfies Record<components["schemas"]["FactorySessionStatus"], string>;

const chineseRuntimeStatusLabels = {
  [FactorySessionStatus.ACTIVE]: "活跃",
  [FactorySessionStatus.FINISHED]: "已完成",
  [FactorySessionStatus.IDLE]: "空闲",
} satisfies Record<components["schemas"]["FactorySessionStatus"], string>;

const englishScriptStatusLabels = {
  [FactorySessionJavaScriptScriptStatus.FAILED]: "Failed",
  [FactorySessionJavaScriptScriptStatus.FINISHED]: "Finished",
  [FactorySessionJavaScriptScriptStatus.IDLE]: "Idle",
  [FactorySessionJavaScriptScriptStatus.PAUSED]: "Paused",
  [FactorySessionJavaScriptScriptStatus.RUNNING]: "Running",
} satisfies Record<
  components["schemas"]["FactorySessionJavaScriptScriptStatus"],
  string
>;

const chineseScriptStatusLabels = {
  [FactorySessionJavaScriptScriptStatus.FAILED]: "失败",
  [FactorySessionJavaScriptScriptStatus.FINISHED]: "已完成",
  [FactorySessionJavaScriptScriptStatus.IDLE]: "空闲",
  [FactorySessionJavaScriptScriptStatus.PAUSED]: "已暂停",
  [FactorySessionJavaScriptScriptStatus.RUNNING]: "运行中",
} satisfies Record<
  components["schemas"]["FactorySessionJavaScriptScriptStatus"],
  string
>;

const englishOrchestratorKindLabels = {
  [FactoryOrchestratorKind.JAVASCRIPT]: "JavaScript workflow",
  [FactoryOrchestratorKind.PETRI]: "Petri net",
} satisfies Record<components["schemas"]["FactoryOrchestratorKind"], string>;

const chineseOrchestratorKindLabels = {
  [FactoryOrchestratorKind.JAVASCRIPT]: "JavaScript 工作流",
  [FactoryOrchestratorKind.PETRI]: "Petri 网",
} satisfies Record<components["schemas"]["FactoryOrchestratorKind"], string>;

const englishDurableLifecycleStatusLabels = {
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusAwaitingApproval]:
    "Awaiting approval",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceling]:
    "Canceling",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceled]:
    "Canceled",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusFailed]:
    "Failed",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusInterrupted]:
    "Interrupted",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusPaused]:
    "Paused",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusQueued]:
    "Queued",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusResuming]:
    "Resuming",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusRunning]:
    "Running",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded]:
    "Succeeded",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTerminated]:
    "Terminated",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTimedOut]:
    "Timed out",
} satisfies Record<
  components["schemas"]["FactorySessionDurableLifecycleStatus"],
  string
>;

const chineseDurableLifecycleStatusLabels = {
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusAwaitingApproval]:
    "等待审批",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceling]:
    "取消中",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusCanceled]:
    "已取消",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusFailed]:
    "失败",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusInterrupted]:
    "已中断",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusPaused]:
    "已暂停",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusQueued]:
    "排队中",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusResuming]:
    "恢复中",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusRunning]:
    "运行中",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded]:
    "已成功",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTerminated]:
    "已终止",
  [FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusTimedOut]:
    "已超时",
} satisfies Record<
  components["schemas"]["FactorySessionDurableLifecycleStatus"],
  string
>;

const factorySessionDetailMessagesByLocale = {
  en: {
    artifactsHeading: "Artifacts",
    checkpointRefsHeading: "Checkpoint refs",
    childDispatchCountsLabel: "Child dispatches",
    dispatchesHeading: "Child dispatch activity",
    durableLifecycleStatusLabels: englishDurableLifecycleStatusLabels,
    dynamicWorkflowShorthand: "Dynamic workflow (JavaScript factory session)",
    enabledTransitionsHeading: "Enabled transitions",
    errorState: "The factory session runtime could not be loaded.",
    finalResultRefLabel: "Final result ref",
    javascriptProjectionMissingState:
      "JavaScript workflow runtime details are not available for this session.",
    loadingState: "Loading factory session runtime…",
    markingEmptyState: "Petri marking: none",
    markingHeading: "Petri marking",
    missingState: "This factory session is no longer available.",
    orchestratorKindLabel: "Orchestrator kind",
    orchestratorKindLabels: englishOrchestratorKindLabels,
    partialResultRefLabel: "Partial result ref",
    phaseLabel: "Phase",
    phasesLabel: "Phases",
    runtimeHeading: "Runtime",
    runtimeStatusLabels: englishRuntimeStatusLabels,
    scriptStatusLabel: "Script status",
    scriptStatusLabels: englishScriptStatusLabels,
    selectedSessionHeading: "Factory session runtime",
    sessionIdLabel: "Session id",
    statusLabel: "Runtime status",
    warningsHeading: "Dispatch warnings",
  },
  "zh-CN": {
    artifactsHeading: "工件",
    checkpointRefsHeading: "检查点引用",
    childDispatchCountsLabel: "子调度",
    dispatchesHeading: "子调度活动",
    durableLifecycleStatusLabels: chineseDurableLifecycleStatusLabels,
    dynamicWorkflowShorthand: "动态工作流（JavaScript 工厂会话）",
    enabledTransitionsHeading: "已启用变迁",
    errorState: "无法加载工厂会话运行时。",
    finalResultRefLabel: "最终结果引用",
    javascriptProjectionMissingState: "此会话的 JavaScript 工作流运行时详情不可用。",
    loadingState: "正在加载工厂会话运行时…",
    markingEmptyState: "Petri 标识：无",
    markingHeading: "Petri 标识",
    missingState: "此工厂会话已不可用。",
    orchestratorKindLabel: "编排器类型",
    orchestratorKindLabels: chineseOrchestratorKindLabels,
    partialResultRefLabel: "部分结果引用",
    phaseLabel: "阶段",
    phasesLabel: "阶段列表",
    runtimeHeading: "运行时",
    runtimeStatusLabels: chineseRuntimeStatusLabels,
    scriptStatusLabel: "脚本状态",
    scriptStatusLabels: chineseScriptStatusLabels,
    selectedSessionHeading: "工厂会话运行时",
    sessionIdLabel: "会话 ID",
    statusLabel: "运行时状态",
    warningsHeading: "调度警告",
  },
} satisfies LocalizedMessageCatalog<FactorySessionDetailMessages>;

export function getFactorySessionDetailMessages(
  locale?: string | null,
): FactorySessionDetailMessages {
  return resolveLocalizedMessages(factorySessionDetailMessagesByLocale, locale);
}
