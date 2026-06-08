import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactorySessionDetailMessages {
  artifactsHeading: string;
  checkpointRefsHeading: string;
  childDispatchCountsLabel: string;
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
