import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactorySessionDetailMessages {
  artifactAuditModeLabel: string;
  artifactCaptureMimeTypeLabel: string;
  artifactCapturedAtLabel: string;
  artifactContentHashLabel: string;
  artifactCreatedAtLabel: string;
  artifactDownloadActionLabel: string;
  artifactDownloadState: string;
  artifactDetailErrorState: string;
  artifactDetailHeading: string;
  artifactDetailLoadingState: string;
  artifactDetailUnavailableState: string;
  artifactDispatchIdLabel: string;
  artifactIdLabel: string;
  artifactKindLabel: string;
  artifactLabelValueLabel: string;
  artifactPreviewHeading: string;
  artifactSizeBytesLabel: string;
  artifactSourceDispatchIdLabel: string;
  artifactsHeading: string;
  artifactSummaryLabel: string;
  artifactViewLabel: string;
  artifactVisibilityLabel: string;
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
    artifactAuditModeLabel: "Audit mode",
    artifactCaptureMimeTypeLabel: "Capture MIME type",
    artifactCapturedAtLabel: "Captured at",
    artifactContentHashLabel: "Content hash",
    artifactCreatedAtLabel: "Created at",
    artifactDownloadActionLabel: "Download artifact",
    artifactDownloadState:
      "Inline preview is unavailable for this durable artifact. Download the artifact to inspect it.",
    artifactDetailErrorState: "The artifact detail could not be loaded.",
    artifactDetailHeading: "Artifact detail",
    artifactDetailLoadingState: "Loading artifact detail…",
    artifactDetailUnavailableState:
      "Inline preview is unavailable for this durable artifact.",
    artifactDispatchIdLabel: "Dispatch id",
    artifactIdLabel: "Artifact id",
    artifactKindLabel: "Kind",
    artifactLabelValueLabel: "Label",
    artifactPreviewHeading: "Preview",
    artifactSizeBytesLabel: "Size (bytes)",
    artifactSourceDispatchIdLabel: "Source dispatch id",
    artifactsHeading: "Artifacts",
    artifactSummaryLabel: "Summary",
    artifactViewLabel: "View artifact",
    artifactVisibilityLabel: "Visibility",
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
    artifactAuditModeLabel: "审计模式",
    artifactCaptureMimeTypeLabel: "捕获 MIME 类型",
    artifactCapturedAtLabel: "捕获时间",
    artifactContentHashLabel: "内容哈希",
    artifactCreatedAtLabel: "创建时间",
    artifactDownloadActionLabel: "下载工件",
    artifactDownloadState: "此持久工件暂不支持内联预览。请下载工件进行查看。",
    artifactDetailErrorState: "无法加载工件详情。",
    artifactDetailHeading: "工件详情",
    artifactDetailLoadingState: "正在加载工件详情…",
    artifactDetailUnavailableState: "此持久工件暂不支持内联预览。",
    artifactDispatchIdLabel: "调度 ID",
    artifactIdLabel: "工件 ID",
    artifactKindLabel: "类型",
    artifactLabelValueLabel: "标签",
    artifactPreviewHeading: "预览",
    artifactSizeBytesLabel: "大小（字节）",
    artifactSourceDispatchIdLabel: "来源调度 ID",
    artifactsHeading: "工件",
    artifactSummaryLabel: "摘要",
    artifactViewLabel: "查看工件",
    artifactVisibilityLabel: "可见性",
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
