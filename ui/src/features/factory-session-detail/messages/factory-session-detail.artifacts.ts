export interface FactorySessionArtifactDetailMessages {
  artifactAuditModeLabel: string;
  artifactCaptureMimeTypeLabel: string;
  artifactCapturedAtLabel: string;
  artifactContentHashLabel: string;
  artifactCreatedAtLabel: string;
  artifactDownloadActionLabel: string;
  artifactDownloadState: string;
  artifactDownloadUnavailableState: string;
  artifactDetailErrorState: string;
  artifactDetailHeading: string;
  artifactDetailLoadingState: string;
  artifactDetailUnavailableState: string;
  artifactDispatchIdLabel: string;
  artifactIdLabel: string;
  artifactKindLabel: string;
  artifactLabelValueLabel: string;
  artifactLinksHeading: string;
  artifactPreviewHeading: string;
  artifactRefActionLabel: string;
  artifactsHeading: string;
  artifactSizeBytesLabel: string;
  artifactSourceDispatchIdLabel: string;
  artifactSummaryLabel: string;
  artifactViewLabel: string;
  artifactVisibilityLabel: string;
}

export const englishFactorySessionArtifactDetailMessages = {
  artifactAuditModeLabel: "Audit mode",
  artifactCaptureMimeTypeLabel: "Capture MIME type",
  artifactCapturedAtLabel: "Captured at",
  artifactContentHashLabel: "Content hash",
  artifactCreatedAtLabel: "Created at",
  artifactDownloadActionLabel: "Download artifact",
  artifactDownloadState:
    "Inline preview is unavailable for this durable artifact. Download the artifact to inspect it.",
  artifactDownloadUnavailableState:
    "Inline preview is unavailable for this durable artifact, and this session detail route does not expose a downloadable payload yet.",
  artifactDetailErrorState: "The artifact detail could not be loaded.",
  artifactDetailHeading: "Artifact detail",
  artifactDetailLoadingState: "Loading artifact detail…",
  artifactDetailUnavailableState:
    "Inline preview is unavailable for this durable artifact.",
  artifactDispatchIdLabel: "Dispatch id",
  artifactIdLabel: "Artifact id",
  artifactKindLabel: "Kind",
  artifactLabelValueLabel: "Label",
  artifactLinksHeading: "Dispatch artifacts",
  artifactPreviewHeading: "Preview",
  artifactRefActionLabel: "Open artifact",
  artifactsHeading: "Artifacts",
  artifactSizeBytesLabel: "Size (bytes)",
  artifactSourceDispatchIdLabel: "Source dispatch id",
  artifactSummaryLabel: "Summary",
  artifactViewLabel: "View artifact",
  artifactVisibilityLabel: "Visibility",
} satisfies FactorySessionArtifactDetailMessages;

export const chineseFactorySessionArtifactDetailMessages = {
  artifactAuditModeLabel: "审计模式",
  artifactCaptureMimeTypeLabel: "捕获 MIME 类型",
  artifactCapturedAtLabel: "捕获时间",
  artifactContentHashLabel: "内容哈希",
  artifactCreatedAtLabel: "创建时间",
  artifactDownloadActionLabel: "下载工件",
  artifactDownloadState: "此持久工件暂不支持内联预览。请下载工件进行查看。",
  artifactDownloadUnavailableState:
    "此持久工件暂不支持内联预览，当前会话详情路由也尚未提供可下载的载荷。",
  artifactDetailErrorState: "无法加载工件详情。",
  artifactDetailHeading: "工件详情",
  artifactDetailLoadingState: "正在加载工件详情…",
  artifactDetailUnavailableState: "此持久工件暂不支持内联预览。",
  artifactDispatchIdLabel: "调度 ID",
  artifactIdLabel: "工件 ID",
  artifactKindLabel: "类型",
  artifactLabelValueLabel: "标签",
  artifactLinksHeading: "调度工件",
  artifactPreviewHeading: "预览",
  artifactRefActionLabel: "打开工件",
  artifactsHeading: "工件",
  artifactSizeBytesLabel: "大小（字节）",
  artifactSourceDispatchIdLabel: "来源调度 ID",
  artifactSummaryLabel: "摘要",
  artifactViewLabel: "查看工件",
  artifactVisibilityLabel: "可见性",
} satisfies FactorySessionArtifactDetailMessages;
