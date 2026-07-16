// biome-ignore-all lint/style/noExcessiveLinesPerFile: localized session inspection copy is kept in one typed catalog.
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
import {
  chineseFactorySessionArtifactDetailMessages,
  englishFactorySessionArtifactDetailMessages,
  type FactorySessionArtifactDetailMessages,
} from "./factory-session-detail.artifacts";
import {
  chineseFactorySessionEventReplayMessages,
  englishFactorySessionEventReplayMessages,
  type FactorySessionEventReplayMessages,
} from "./factory-session-detail.event-replay";
import {
  chineseFactorySessionLifecycleOutcomeMessages,
  englishFactorySessionLifecycleOutcomeMessages,
  type FactorySessionLifecycleOutcomeMessages,
} from "./factory-session-detail.lifecycle-outcomes";

export interface FactorySessionDetailMessages
  extends FactorySessionArtifactDetailMessages,
    FactorySessionEventReplayMessages,
    FactorySessionLifecycleOutcomeMessages {
  budgetLabel: string;
  lifecycleActionApproveLabel: string;
  lifecycleActionCancelLabel: string;
  lifecycleActionInterruptDispatchLabel: string;
  lifecycleActionPauseLabel: string;
  lifecycleActionResumeLabel: string;
  lifecycleActionRetryDispatchLabel: string;
  lifecycleActionTerminateLabel: string;
  lifecycleControlsEmptyState: string;
  lifecycleControlsHeading: string;
  lifecycleControlsRetrySelectionHint: string;
  lifecycleControlsSelectedDispatchLabel: (dispatchID: string) => string;
  checkpointRefsHeading: string;
  childDispatchCountsLabel: string;
  collapseDispatchDetailLabel: (dispatchID: string) => string;
  dispatchAttemptLabel: string;
  dispatchDetailErrorState: string;
  dispatchDetailHeading: string;
  dispatchDetailMissingState: (dispatchID: string) => string;
  dispatchDetailLoadingState: string;
  dispatchDetailRetryLabel: string;
  dispatchExecutionModeSummary: (mode: string) => string;
  dispatchKindLabel: string;
  dispatchLabelField: string;
  dispatchProviderSessionSummary: (input: {
    id: string;
    kind: string;
    provider?: string;
  }) => string;
  dispatchSelectionHint: string;
  dispatchStatusLabel: string;
  dispatchesHeading: string;
  dispatchCountsLabel: string;
  durableLifecycleStatusLabels: Record<
    components["schemas"]["FactorySessionDurableLifecycleStatus"],
    string
  >;
  dynamicWorkflowShorthand: string;
  executionModeLabel: string;
  enabledTransitionsHeading: string;
  errorState: string;
  expandDispatchDetailLabel: (dispatchID: string) => string;
  failureDetailHeading: string;
  failureErrorClassLabel: string;
  failureMessageLabel: string;
  failureReasonLabel: string;
  finalResultRefLabel: string;
  effectivePolicyLabel: string;
  introspectionHeading: string;
  latestCheckpointLabel: string;
  lifecycleControlStatusLabel: string;
  javascriptProjectionMissingState: string;
  javascriptTaskHeading: string;
  javascriptTaskKindLabel: string;
  javascriptTaskLabel: string;
  loadingState: string;
  markingEmptyState: string;
  markingHeading: string;
  missingState: string;
  modelLabel: string;
  orchestratorKindLabel: string;
  orchestratorKindLabels: Record<
    components["schemas"]["FactoryOrchestratorKind"],
    string
  >;
  partialResultRefLabel: string;
  phaseDispatchSummary: (counts: {
    completed: number;
    failed: number;
    total: number;
  }) => string;
  phaseSummariesHeading: string;
  currentPhaseValue: string;
  noneValue: string;
  resultAvailabilityLabel: string;
  resultAvailabilityValue: (status: string) => string;
  sourceHashLabel: string;
  sourceLabel: string;
  unavailableValue: string;
  phaseLabel: string;
  phasesLabel: string;
  petriDetailHeading: string;
  petriTransitionLabel: string;
  petriWorkerTypeLabel: string;
  petriWorkstationLabel: string;
  promptDigestLabel: string;
  providerLabel: string;
  providerSessionHeading: string;
  providerSessionRefLabel: string;
  relatedWorkHeading: string;
  relatedWorkLabel: string;
  runnerIdLabel: string;
  runtimeHeading: string;
  runtimeStatusLabels: Record<
    components["schemas"]["FactorySessionStatus"],
    string
  >;
  schemaDigestLabel: string;
  scriptStatusLabel: string;
  scriptStatusLabels: Record<
    components["schemas"]["FactorySessionJavaScriptScriptStatus"],
    string
  >;
  selectedSessionHeading: string;
  sessionIdLabel: string;
  statusHistoryHeading: string;
  statusHistoryLabel: string;
  statusLabel: string;
  usageHeading: string;
  usageCostLabel: string;
  usageDurationLabel: string;
  usageInputTokensLabel: string;
  usageOutputTokensLabel: string;
  usageRetryCountLabel: string;
  usageTotalTokensLabel: string;
  warningCodeLabel: string;
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
    ...englishFactorySessionArtifactDetailMessages,
    budgetLabel: "Effective budget",
    lifecycleActionApproveLabel: "Approve",
    lifecycleActionCancelLabel: "Cancel",
    lifecycleActionInterruptDispatchLabel: "Interrupt dispatch",
    lifecycleActionPauseLabel: "Pause",
    lifecycleActionResumeLabel: "Resume",
    lifecycleActionRetryDispatchLabel: "Retry dispatch",
    lifecycleActionTerminateLabel: "Terminate",
    ...englishFactorySessionLifecycleOutcomeMessages,
    lifecycleControlsEmptyState:
      "No lifecycle controls are available for this Factory Session state.",
    lifecycleControlsHeading: "Lifecycle controls",
    lifecycleControlsRetrySelectionHint:
      "Select a running or failed dispatch to make interrupt or retry available on this detail surface.",
    lifecycleControlsSelectedDispatchLabel: (dispatchID) =>
      `Selected dispatch: ${dispatchID}`,
    checkpointRefsHeading: "Checkpoint refs",
    collapseDispatchDetailLabel: (dispatchID) =>
      `Collapse dispatch detail for ${dispatchID}`,
    childDispatchCountsLabel: "Child dispatches",
    dispatchAttemptLabel: "Attempt",
    dispatchDetailErrorState: "The dispatch detail could not be loaded.",
    dispatchDetailHeading: "Dispatch detail",
    dispatchDetailLoadingState: "Loading dispatch detail…",
    dispatchDetailMissingState: (dispatchID) =>
      `Dispatch detail for ${dispatchID} is no longer available.`,
    dispatchDetailRetryLabel: "Retry loading dispatch detail",
    dispatchExecutionModeSummary: (mode) => `Execution mode: ${mode}`,
    dispatchKindLabel: "Dispatch kind",
    dispatchLabelField: "Dispatch label",
    dispatchProviderSessionSummary: ({ id, kind, provider }) =>
      provider
        ? `Provider session: ${provider} / ${kind} / ${id}`
        : `Provider session: ${kind} / ${id}`,
    dispatchSelectionHint:
      "Select a dispatch to inspect bounded durable detail.",
    dispatchStatusLabel: "Dispatch status",
    dispatchesHeading: "Dispatches",
    dispatchCountsLabel: "Dispatch counts by status",
    durableLifecycleStatusLabels: englishDurableLifecycleStatusLabels,
    dynamicWorkflowShorthand: "Dynamic workflow (JavaScript factory session)",
    executionModeLabel: "Execution mode",
    enabledTransitionsHeading: "Enabled transitions",
    ...englishFactorySessionEventReplayMessages,
    errorState: "The factory session runtime could not be loaded.",
    expandDispatchDetailLabel: (dispatchID) =>
      `Expand dispatch detail for ${dispatchID}`,
    failureDetailHeading: "Failure detail",
    failureErrorClassLabel: "Error class",
    failureMessageLabel: "Failure message",
    failureReasonLabel: "Failure reason",
    finalResultRefLabel: "Final result ref",
    effectivePolicyLabel: "Effective policy",
    introspectionHeading: "Session introspection",
    latestCheckpointLabel: "Latest checkpoint",
    lifecycleControlStatusLabel: "Factory Session lifecycle",
    javascriptProjectionMissingState:
      "JavaScript workflow runtime details are not available for this session.",
    javascriptTaskHeading: "JavaScript task",
    javascriptTaskKindLabel: "Task kind",
    javascriptTaskLabel: "Task label",
    loadingState: "Loading factory session runtime…",
    markingEmptyState: "Petri marking: none",
    markingHeading: "Petri marking",
    missingState: "This factory session is no longer available.",
    modelLabel: "Model",
    orchestratorKindLabel: "Orchestrator kind",
    orchestratorKindLabels: englishOrchestratorKindLabels,
    partialResultRefLabel: "Partial result ref",
    phaseDispatchSummary: ({ completed, failed, total }) =>
      `${total} dispatches · ${completed} completed · ${failed} failed`,
    phaseSummariesHeading: "Phase progress",
    currentPhaseValue: "current",
    noneValue: "None",
    resultAvailabilityLabel: "Result availability",
    resultAvailabilityValue: (status) => status.toLowerCase(),
    sourceHashLabel: "Source hash",
    sourceLabel: "Source",
    unavailableValue: "Unavailable",
    phaseLabel: "Phase",
    phasesLabel: "Phases",
    petriDetailHeading: "Petri dispatch",
    petriTransitionLabel: "Transition id",
    petriWorkerTypeLabel: "Worker type",
    petriWorkstationLabel: "Workstation",
    promptDigestLabel: "Prompt digest",
    providerLabel: "Provider",
    providerSessionHeading: "Provider sessions",
    providerSessionRefLabel: "Provider ref",
    relatedWorkHeading: "Related work",
    relatedWorkLabel: "Work id",
    runnerIdLabel: "Runner id",
    runtimeHeading: "Runtime",
    runtimeStatusLabels: englishRuntimeStatusLabels,
    schemaDigestLabel: "Schema digest",
    scriptStatusLabel: "Script status",
    scriptStatusLabels: englishScriptStatusLabels,
    selectedSessionHeading: "Factory session runtime",
    sessionIdLabel: "Session id",
    statusHistoryHeading: "Status history",
    statusHistoryLabel: "Status",
    statusLabel: "Runtime status",
    usageHeading: "Usage",
    usageCostLabel: "Cost (USD)",
    usageDurationLabel: "Duration",
    usageInputTokensLabel: "Input tokens",
    usageOutputTokensLabel: "Output tokens",
    usageRetryCountLabel: "Retry count",
    usageTotalTokensLabel: "Total tokens",
    warningCodeLabel: "Warning code",
    warningsHeading: "Dispatch warnings",
  },
  "zh-CN": {
    ...chineseFactorySessionArtifactDetailMessages,
    budgetLabel: "有效预算",
    lifecycleActionApproveLabel: "批准",
    lifecycleActionCancelLabel: "取消",
    lifecycleActionInterruptDispatchLabel: "中断调度",
    lifecycleActionPauseLabel: "暂停",
    lifecycleActionResumeLabel: "恢复",
    lifecycleActionRetryDispatchLabel: "重试调度",
    lifecycleActionTerminateLabel: "终止",
    ...chineseFactorySessionLifecycleOutcomeMessages,
    lifecycleControlsEmptyState: "当前工厂会话状态没有可用的生命周期控制。",
    lifecycleControlsHeading: "生命周期控制",
    lifecycleControlsRetrySelectionHint:
      "选择运行中或失败的调度后，当前详情界面才会显示中断或重试操作。",
    lifecycleControlsSelectedDispatchLabel: (dispatchID) =>
      `已选择调度：${dispatchID}`,
    checkpointRefsHeading: "检查点引用",
    collapseDispatchDetailLabel: (dispatchID) =>
      `收起 ${dispatchID} 的调度详情`,
    childDispatchCountsLabel: "子调度",
    dispatchAttemptLabel: "尝试次数",
    dispatchDetailErrorState: "无法加载调度详情。",
    dispatchDetailHeading: "调度详情",
    dispatchDetailLoadingState: "正在加载调度详情…",
    dispatchDetailMissingState: (dispatchID) =>
      `${dispatchID} 的调度详情已不可用。`,
    dispatchDetailRetryLabel: "重试加载调度详情",
    dispatchExecutionModeSummary: (mode) => `执行模式：${mode}`,
    dispatchKindLabel: "调度类型",
    dispatchLabelField: "调度标签",
    dispatchProviderSessionSummary: ({ id, kind, provider }) =>
      provider
        ? `Provider session：${provider} / ${kind} / ${id}`
        : `Provider session：${kind} / ${id}`,
    dispatchSelectionHint: "选择一个调度以检查受限的持久化详情。",
    dispatchStatusLabel: "调度状态",
    dispatchesHeading: "调度",
    dispatchCountsLabel: "按状态统计的调度数",
    durableLifecycleStatusLabels: chineseDurableLifecycleStatusLabels,
    dynamicWorkflowShorthand: "动态工作流（JavaScript 工厂会话）",
    executionModeLabel: "执行模式",
    enabledTransitionsHeading: "已启用变迁",
    ...chineseFactorySessionEventReplayMessages,
    errorState: "无法加载工厂会话运行时。",
    expandDispatchDetailLabel: (dispatchID) => `展开 ${dispatchID} 的调度详情`,
    failureDetailHeading: "失败详情",
    failureErrorClassLabel: "错误类别",
    failureMessageLabel: "失败消息",
    failureReasonLabel: "失败原因",
    finalResultRefLabel: "最终结果引用",
    effectivePolicyLabel: "有效策略",
    introspectionHeading: "会话检查",
    latestCheckpointLabel: "最新检查点",
    lifecycleControlStatusLabel: "工厂会话生命周期",
    javascriptProjectionMissingState:
      "此会话的 JavaScript 工作流运行时详情不可用。",
    javascriptTaskHeading: "JavaScript 任务",
    javascriptTaskKindLabel: "任务类型",
    javascriptTaskLabel: "任务标签",
    loadingState: "正在加载工厂会话运行时…",
    markingEmptyState: "Petri 标识：无",
    markingHeading: "Petri 标识",
    missingState: "此工厂会话已不可用。",
    modelLabel: "模型",
    orchestratorKindLabel: "编排器类型",
    orchestratorKindLabels: chineseOrchestratorKindLabels,
    partialResultRefLabel: "部分结果引用",
    phaseDispatchSummary: ({ completed, failed, total }) =>
      `${total} 个调度 · ${completed} 个已完成 · ${failed} 个失败`,
    phaseSummariesHeading: "阶段进度",
    currentPhaseValue: "当前",
    noneValue: "无",
    resultAvailabilityLabel: "结果可用性",
    resultAvailabilityValue: (status) => status.toLowerCase(),
    sourceHashLabel: "源哈希",
    sourceLabel: "来源",
    unavailableValue: "不可用",
    phaseLabel: "阶段",
    phasesLabel: "阶段列表",
    petriDetailHeading: "Petri 调度",
    petriTransitionLabel: "变迁 ID",
    petriWorkerTypeLabel: "工作器类型",
    petriWorkstationLabel: "工作站",
    promptDigestLabel: "提示摘要",
    providerLabel: "提供方",
    providerSessionHeading: "提供方会话",
    providerSessionRefLabel: "提供方引用",
    relatedWorkHeading: "关联工作",
    relatedWorkLabel: "工作 ID",
    runnerIdLabel: "运行器 ID",
    runtimeHeading: "运行时",
    runtimeStatusLabels: chineseRuntimeStatusLabels,
    schemaDigestLabel: "Schema 摘要",
    scriptStatusLabel: "脚本状态",
    scriptStatusLabels: chineseScriptStatusLabels,
    selectedSessionHeading: "工厂会话运行时",
    sessionIdLabel: "会话 ID",
    statusHistoryHeading: "状态历史",
    statusHistoryLabel: "状态",
    statusLabel: "运行时状态",
    usageHeading: "用量",
    usageCostLabel: "费用（USD）",
    usageDurationLabel: "持续时间",
    usageInputTokensLabel: "输入令牌",
    usageOutputTokensLabel: "输出令牌",
    usageRetryCountLabel: "重试次数",
    usageTotalTokensLabel: "总令牌",
    warningCodeLabel: "警告代码",
    warningsHeading: "调度警告",
  },
} satisfies LocalizedMessageCatalog<FactorySessionDetailMessages>;

export function getFactorySessionDetailMessages(
  locale?: string | null,
): FactorySessionDetailMessages {
  return resolveLocalizedMessages(factorySessionDetailMessagesByLocale, locale);
}
