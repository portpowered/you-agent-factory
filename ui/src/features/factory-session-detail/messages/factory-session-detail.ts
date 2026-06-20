import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactorySessionDetailMessages {
  artifactsHeading: string;
  checkpointRefsHeading: string;
  childDispatchCountsLabel: string;
  durableAvailabilityLabel: string;
  durableDetailRegionLabel: string;
  durableErrorState: string;
  durableErrorTitle: string;
  durableFailureLabel: string;
  durableLoadingState: string;
  durableLoadingTitle: string;
  durableMissingState: string;
  durableMissingTitle: string;
  durablePartialArtifactRefsHeading: string;
  durablePartialState: string;
  durablePartialTitle: string;
  durableResultStatusLabel: string;
  durableTerminalArtifactRefsHeading: string;
  durableTerminalState: string;
  durableTerminalTitle: string;
  dynamicWorkflowShorthand: string;
  enabledTransitionsHeading: string;
  errorState: string;
  finalResultRefLabel: string;
  loadingState: string;
  markingEmptyState: string;
  markingHeading: string;
  missingState: string;
  orchestratorKindLabel: string;
  partialResultRefLabel: string;
  phaseLabel: string;
  phasesLabel: string;
  progressLabel: string;
  resolvedSourceLabel: string;
  resultSummaryLabel: string;
  runtimeHeading: string;
  scriptStatusLabel: string;
  selectedSessionHeading: string;
  sessionIdLabel: string;
  statusLabel: string;
  warningsHeading: string;
}

const factorySessionDetailMessagesByLocale = {
  en: {
    artifactsHeading: "Artifacts",
    checkpointRefsHeading: "Checkpoint refs",
    childDispatchCountsLabel: "Child dispatches",
    durableAvailabilityLabel: "Result availability",
    durableDetailRegionLabel: "Factory Session detail",
    durableErrorState:
      "The Factory Session detail request failed. Retry the selection or check factory session API availability.",
    durableErrorTitle: "Factory Session detail unavailable",
    durableFailureLabel: "Failure detail",
    durableLoadingState: "Loading Factory Session detail from durable reads…",
    durableLoadingTitle: "Loading Factory Session detail",
    durableMissingState:
      "This Factory Session is not available. It may have been removed or the id is incorrect.",
    durableMissingTitle: "Factory Session not found",
    durablePartialArtifactRefsHeading: "Partial result artifact refs",
    durablePartialState:
      "This Factory Session has not produced a final result yet. Partial inspection is shown below.",
    durablePartialTitle: "Factory Session in progress",
    durableResultStatusLabel: "Result status",
    durableTerminalArtifactRefsHeading: "Final result artifact refs",
    durableTerminalState:
      "This Factory Session reached a terminal state. Final inspection is shown below.",
    durableTerminalTitle: "Factory Session complete",
    dynamicWorkflowShorthand: "Dynamic workflow (JavaScript factory session)",
    enabledTransitionsHeading: "Enabled transitions",
    errorState: "The factory session runtime could not be loaded.",
    finalResultRefLabel: "Final result ref",
    loadingState: "Loading factory session runtime…",
    markingEmptyState: "Petri marking: none",
    markingHeading: "Petri marking",
    missingState: "This factory session is no longer available.",
    orchestratorKindLabel: "Orchestrator kind",
    partialResultRefLabel: "Partial result ref",
    phaseLabel: "Phase",
    phasesLabel: "Phases",
    progressLabel: "Progress",
    resolvedSourceLabel: "Resolved source",
    resultSummaryLabel: "Result summary",
    runtimeHeading: "Runtime",
    scriptStatusLabel: "Script status",
    selectedSessionHeading: "Factory session runtime",
    sessionIdLabel: "Session id",
    statusLabel: "Runtime status",
    warningsHeading: "Dispatch warnings",
  },
  "zh-CN": {
    artifactsHeading: "工件",
    checkpointRefsHeading: "检查点引用",
    childDispatchCountsLabel: "子调度",
    durableAvailabilityLabel: "结果可用性",
    durableDetailRegionLabel: "工厂会话详情",
    durableErrorState:
      "工厂会话详情请求失败。请重试选择或检查工厂会话 API 是否可用。",
    durableErrorTitle: "工厂会话详情不可用",
    durableFailureLabel: "失败详情",
    durableLoadingState: "正在从持久化读取加载工厂会话详情…",
    durableLoadingTitle: "正在加载工厂会话详情",
    durableMissingState:
      "此工厂会话不可用。它可能已被移除，或标识符不正确。",
    durableMissingTitle: "未找到工厂会话",
    durablePartialArtifactRefsHeading: "部分结果工件引用",
    durablePartialState:
      "此工厂会话尚未产生最终结果。下方显示部分检查信息。",
    durablePartialTitle: "工厂会话进行中",
    durableResultStatusLabel: "结果状态",
    durableTerminalArtifactRefsHeading: "最终结果工件引用",
    durableTerminalState:
      "此工厂会话已达到终止状态。下方显示最终检查信息。",
    durableTerminalTitle: "工厂会话已完成",
    dynamicWorkflowShorthand: "动态工作流（JavaScript 工厂会话）",
    enabledTransitionsHeading: "已启用变迁",
    errorState: "无法加载工厂会话运行时。",
    finalResultRefLabel: "最终结果引用",
    loadingState: "正在加载工厂会话运行时…",
    markingEmptyState: "Petri 标识：无",
    markingHeading: "Petri 标识",
    missingState: "此工厂会话已不可用。",
    orchestratorKindLabel: "编排器类型",
    partialResultRefLabel: "部分结果引用",
    phaseLabel: "阶段",
    phasesLabel: "阶段列表",
    progressLabel: "进度",
    resolvedSourceLabel: "已解析来源",
    resultSummaryLabel: "结果摘要",
    runtimeHeading: "运行时",
    scriptStatusLabel: "脚本状态",
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
