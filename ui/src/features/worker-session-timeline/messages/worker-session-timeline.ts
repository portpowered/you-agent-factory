// biome-ignore-all lint/style/noExcessiveLinesPerFile: feature-local locale catalogs keep required language coverage in one typed asset set.
import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type {
  WorkerTimelineEntryCategory,
  WorkerTimelineTerminalOutcome,
} from "../lib/worker-session-timeline-projection-types";

export type WorkerSessionTimelineRecordingHealth =
  | "COMPLETE"
  | "DEGRADED"
  | "INCOMPLETE";

export type WorkerSessionTimelineViewStatus =
  | "completed"
  | "empty"
  | "idle"
  | "live"
  | "loading"
  | "reconnecting"
  | "source-error";

export interface WorkerSessionTimelineMessages {
  attemptIDLabel: string;
  attemptLabel: (number: number | undefined, id: string | undefined) => string;
  ariaLabel: string;
  cacheWriteTokensLabel: string;
  cachedInputTokensLabel: string;
  collapseDetailsLabel: string;
  collapseContentAction: string;
  completedState: string;
  continuationLabel: string;
  detailsLabel: (expanded: boolean) => string;
  dispatchLabel: string;
  durabilityConfirmationLabel: string;
  earlierEventsAction: string;
  emptyState: string;
  eventListLabel: string;
  eventPositionLabel: (position: number) => string;
  expandContentAction: string;
  failureLabel: string;
  genericMetadataLabel: string;
  genericSchemaLabel: (schemaID: string) => string;
  idleState: string;
  incompleteRecordingNotice: (reason?: string | null) => string;
  inputTokensLabel: string;
  loadingState: string;
  liveFollowingState: string;
  messageRoleLabel: (role: string) => string;
  messageTextLabel: string;
  laterEventsAction: string;
  modelLabel: string;
  modelProviderLabel: string;
  newActivityAction: (count: number) => string;
  noDetailState: string;
  openWorkerSessionLabel: (workerSessionID: string) => string;
  outputTokensLabel: string;
  predecessorLabel: string;
  providerLabel: string;
  providerSessionLabel: string;
  reasoningDeltaLabel: string;
  reasoningLabel: string;
  reasoningOutputTokensLabel: string;
  reasoningSnapshotLabel: string;
  reconnectingState: string;
  recordingHealthLabel: (
    health: WorkerSessionTimelineRecordingHealth,
  ) => string;
  recordingHealthPending: string;
  recordingHealthNotice: (
    health: WorkerSessionTimelineRecordingHealth,
    reason?: string | null,
  ) => string;
  retryAction: string;
  retryAfterSecondsLabel: string;
  retryableLabel: string;
  retryReasonLabel: string;
  openWorkerSessionAction: string;
  openWorkerSessionTargetLabel: (
    workerSessionID: string,
    workID: string | null,
  ) => string;
  providerSessionUnavailable: string;
  recordingHealthReasonLabel: string;
  recordingHealthTargetLabel: string;
  progressLabel: string;
  progressMessageLabel: string;
  progressPercentLabel: string;
  selectedWorkerSessionAction: string;
  sessionLifecycleLabel: string;
  sessionLifecycleStateLabel: (state: string) => string;
  workerSessionIDLabel: string;
  attemptDispatchIDLabel: string;
  durationLabel: string;
  durationBasisLabel: string;
  durationValue: (durationMillis: number | null) => string;
  failureSummaryLabel: string;
  modelUnavailable: string;
  workIDsLabel: string;
  workLabel: string;
  reasoningEffortLabel: string;
  sourceErrorHeading: string;
  sourceLabel: string;
  sourceSequenceLabel: (sequence: number) => string;
  sessionTargetEmpty: string;
  sessionTargetError: string;
  sessionTargetLoading: string;
  sessionTargetListLabel: string;
  sessionTargetRetry: string;
  successorLabel: string;
  terminalOutcomeLabel: (outcome: WorkerTimelineTerminalOutcome) => string;
  terminalOutcomeHeading: string;
  timelineTitle: string;
  windowNavigationLabel: string;
  windowRangeLabel: (start: number, end: number, total: number) => string;
  toolArgumentsLabel: string;
  toolCallIDLabel: string;
  toolLabel: string;
  toolNameLabel: string;
  toolOutputLabel: string;
  toolResultLabel: string;
  toolStatusLabel: string;
  totalTokensLabel: string;
  turnLabel: string;
  unknownValueLabel: string;
  unknownCategoryLabel: string;
  usageLabel: string;
  usageModelLabel: string;
  workSelectionRequired: string;
  categoryLabel: Record<WorkerTimelineEntryCategory, string>;
  phaseLabel: (phase: string) => string;
}

const workerSessionTimelineMessagesByLocale: LocalizedMessageCatalog<WorkerSessionTimelineMessages> =
  {
    en: {
      attemptIDLabel: "Attempt ID",
      attemptDispatchIDLabel: "Attempt / dispatch ID",
      attemptLabel: (number, id) =>
        number === undefined && id === undefined
          ? "Attempt"
          : `Attempt ${number ?? "unknown"}${id ? ` (${id})` : ""}`,
      ariaLabel: "Worker Session timeline",
      cacheWriteTokensLabel: "Cache write tokens",
      cachedInputTokensLabel: "Cached input tokens",
      collapseDetailsLabel: "Collapse details",
      collapseContentAction: "Hide full content",
      completedState: "Worker Session delivery is complete.",
      continuationLabel: "Continuation",
      detailsLabel: (expanded) => (expanded ? "Hide details" : "Show details"),
      dispatchLabel: "Dispatch",
      durabilityConfirmationLabel: "Durability confirmation",
      earlierEventsAction: "Earlier events",
      emptyState: "No canonical Worker Session records were retained.",
      eventListLabel: "Canonical Worker Session events",
      eventPositionLabel: (position) => `Canonical position ${position}`,
      expandContentAction: "Show full content",
      failureLabel: "Failure",
      genericMetadataLabel: "Forward-compatible event metadata",
      genericSchemaLabel: (schemaID) => `Unknown schema ${schemaID}`,
      idleState: "Select a Worker Session to inspect its timeline.",
      incompleteRecordingNotice: (reason) =>
        reason
          ? `Some retained history may be missing. Recording reason: ${reason}.`
          : "Some retained history may be missing.",
      inputTokensLabel: "Input tokens",
      loadingState: "Loading Worker Session records…",
      liveFollowingState: "Following live Worker Session activity.",
      messageRoleLabel: (role) => `Role: ${role}`,
      messageTextLabel: "Message",
      laterEventsAction: "Later events",
      modelLabel: "Model",
      modelProviderLabel: "Model provider",
      newActivityAction: (count) =>
        `View ${count} new ${count === 1 ? "event" : "events"}`,
      noDetailState:
        "This canonical event has no additional displayable detail.",
      openWorkerSessionLabel: (workerSessionID) =>
        `Open Worker Session ${workerSessionID}`,
      openWorkerSessionAction: "Open Worker Session",
      openWorkerSessionTargetLabel: (workerSessionID, workID) =>
        workID
          ? `Open Worker Session ${workerSessionID} for Work ${workID}`
          : `Open Worker Session ${workerSessionID}`,
      outputTokensLabel: "Output tokens",
      predecessorLabel: "Predecessor Worker Session",
      providerLabel: "Provider",
      providerSessionLabel: "Provider Session",
      providerSessionUnavailable: "Provider Session unavailable",
      recordingHealthReasonLabel: "Recording health reason",
      recordingHealthTargetLabel: "Recording health",
      progressLabel: "Progress",
      progressMessageLabel: "Progress message",
      progressPercentLabel: "Progress percent",
      reasoningDeltaLabel: "Reasoning delta",
      reasoningLabel: "Reasoning",
      reasoningEffortLabel: "Reasoning effort",
      reasoningOutputTokensLabel: "Reasoning output tokens",
      reasoningSnapshotLabel: "Reasoning snapshot",
      reconnectingState: "Worker Session activity is reconnecting.",
      recordingHealthLabel: (health) => `Recording: ${health}`,
      recordingHealthPending: "Recording health pending",
      recordingHealthNotice: (health, reason) =>
        health === "COMPLETE"
          ? "Retained recording is complete. This does not imply execution success."
          : health === "DEGRADED"
            ? reason
              ? `Recording is degraded; some history may be unavailable. Reason: ${reason}.`
              : "Recording is degraded; some history may be unavailable."
            : reason
              ? `Recording is incomplete; some history may be missing. Reason: ${reason}.`
              : "Recording is incomplete; some history may be missing.",
      retryAction: "Retry",
      retryAfterSecondsLabel: "Retry after seconds",
      retryableLabel: "Retryable",
      retryReasonLabel: "Retry reason",
      selectedWorkerSessionAction: "Worker Session selected",
      sessionLifecycleLabel: "Lifecycle outcome",
      sessionLifecycleStateLabel: (state) => {
        switch (state.trim().toUpperCase()) {
          case "CANCELED":
          case "CANCELLED":
            return "Canceled";
          case "COMPLETED":
            return "Completed";
          case "FAILED":
            return "Failed";
          case "RUNNING":
            return "Running";
          case "STARTING":
            return "Starting";
          default:
            return state || "Unknown";
        }
      },
      workerSessionIDLabel: "Worker Session ID",
      sourceErrorHeading: "Worker Session records could not be loaded",
      sourceLabel: "Source",
      sourceSequenceLabel: (sequence) => `Source sequence ${sequence}`,
      sessionTargetEmpty:
        "No Worker Session attempts are correlated with this Work.",
      sessionTargetError: "Worker Session attempts could not be loaded.",
      sessionTargetLoading: "Loading Worker Session attempts…",
      sessionTargetListLabel: "Worker Session attempts",
      sessionTargetRetry: "Retry Worker Session lookup",
      successorLabel: "Successor Worker Session",
      terminalOutcomeLabel: (outcome) =>
        outcome === "SUCCESS"
          ? "Succeeded"
          : outcome === "FAILURE"
            ? "Failed"
            : "Canceled",
      terminalOutcomeHeading: "Terminal outcome",
      timelineTitle: "Worker Session timeline",
      windowNavigationLabel: "Timeline window navigation",
      windowRangeLabel: (start, end, total) =>
        `Events ${start}–${end} of ${total}`,
      toolArgumentsLabel: "Arguments",
      toolCallIDLabel: "Tool call ID",
      toolLabel: "Tool",
      toolNameLabel: "Tool name",
      toolOutputLabel: "Tool output",
      toolResultLabel: "Result",
      toolStatusLabel: "Tool status",
      totalTokensLabel: "Total tokens",
      turnLabel: "Turn",
      unknownValueLabel: "Unknown",
      unknownCategoryLabel: "Canonical event",
      usageLabel: "Usage",
      usageModelLabel: "Usage model",
      durationLabel: "Duration",
      durationBasisLabel: "Duration basis",
      durationValue: (durationMillis) =>
        durationMillis === null ? "Unknown" : `${durationMillis} ms`,
      failureSummaryLabel: "Failure summary",
      modelUnavailable: "Model unavailable",
      workIDsLabel: "Work IDs",
      workLabel: "Work",
      workSelectionRequired:
        "Select a Work item to inspect its Worker Session timeline.",
      categoryLabel: {
        error: "Error",
        "file-change": "File change",
        generic: "Canonical event",
        message: "Message",
        plan: "Plan",
        progress: "Progress",
        reasoning: "Reasoning",
        run: "Run",
        session: "Session",
        "stream-gap": "Stream gap",
        tool: "Tool",
        turn: "Turn",
        usage: "Usage",
      },
      phaseLabel: (phase) => phase,
    },
    "zh-CN": {
      attemptIDLabel: "尝试 ID",
      attemptDispatchIDLabel: "尝试 / 分派 ID",
      attemptLabel: (number, id) =>
        number === undefined && id === undefined
          ? "尝试"
          : `尝试 ${number ?? "未知"}${id ? `（${id}）` : ""}`,
      ariaLabel: "Worker 会话时间线",
      cacheWriteTokensLabel: "缓存写入令牌",
      cachedInputTokensLabel: "缓存输入令牌",
      collapseDetailsLabel: "折叠详情",
      collapseContentAction: "隐藏完整内容",
      completedState: "Worker 会话记录已传递完成。",
      continuationLabel: "继续关系",
      detailsLabel: (expanded) => (expanded ? "隐藏详情" : "显示详情"),
      dispatchLabel: "分派",
      durabilityConfirmationLabel: "持久化确认",
      earlierEventsAction: "更早的事件",
      emptyState: "没有保留的规范 Worker 会话记录。",
      eventListLabel: "规范 Worker 会话事件",
      eventPositionLabel: (position) => `规范位置 ${position}`,
      expandContentAction: "显示完整内容",
      failureLabel: "失败",
      genericMetadataLabel: "前向兼容事件元数据",
      genericSchemaLabel: (schemaID) => `未知架构 ${schemaID}`,
      idleState: "选择一个 Worker 会话以查看其时间线。",
      incompleteRecordingNotice: (reason) =>
        reason
          ? `部分保留历史可能缺失。记录原因：${reason}。`
          : "部分保留历史可能缺失。",
      inputTokensLabel: "输入令牌",
      loadingState: "正在加载 Worker 会话记录…",
      liveFollowingState: "正在跟踪 Worker 会话实时活动。",
      messageRoleLabel: (role) => `角色：${role}`,
      messageTextLabel: "消息",
      laterEventsAction: "更晚的事件",
      modelLabel: "模型",
      modelProviderLabel: "模型提供方",
      newActivityAction: (count) => `查看 ${count} 个新事件`,
      noDetailState: "此规范事件没有可显示的其他详情。",
      openWorkerSessionLabel: (workerSessionID) =>
        `打开 Worker 会话 ${workerSessionID}`,
      openWorkerSessionAction: "打开 Worker 会话",
      openWorkerSessionTargetLabel: (workerSessionID, workID) =>
        workID
          ? `打开工作 ${workID} 的 Worker 会话 ${workerSessionID}`
          : `打开 Worker 会话 ${workerSessionID}`,
      outputTokensLabel: "输出令牌",
      predecessorLabel: "前置 Worker 会话",
      providerLabel: "提供方",
      providerSessionLabel: "提供方会话",
      providerSessionUnavailable: "提供方会话不可用",
      recordingHealthReasonLabel: "记录健康原因",
      recordingHealthTargetLabel: "记录健康状态",
      progressLabel: "进度",
      progressMessageLabel: "进度消息",
      progressPercentLabel: "进度百分比",
      reasoningDeltaLabel: "推理增量",
      reasoningLabel: "推理",
      reasoningEffortLabel: "推理强度",
      reasoningOutputTokensLabel: "推理输出令牌",
      reasoningSnapshotLabel: "推理快照",
      reconnectingState: "Worker 会话活动正在重新连接。",
      recordingHealthLabel: (health) => `记录：${health}`,
      recordingHealthPending: "记录健康状态待定",
      recordingHealthNotice: (health, reason) =>
        health === "COMPLETE"
          ? "保留记录完整。这不代表执行成功。"
          : health === "DEGRADED"
            ? reason
              ? `记录已降级，部分历史可能不可用。原因：${reason}。`
              : "记录已降级，部分历史可能不可用。"
            : reason
              ? `记录不完整，部分历史可能缺失。原因：${reason}。`
              : "记录不完整，部分历史可能缺失。",
      retryAction: "重试",
      retryAfterSecondsLabel: "重试等待秒数",
      retryableLabel: "可重试",
      retryReasonLabel: "重试原因",
      selectedWorkerSessionAction: "Worker 会话已选择",
      sessionLifecycleLabel: "生命周期结果",
      sessionLifecycleStateLabel: (state) => {
        switch (state.trim().toUpperCase()) {
          case "CANCELED":
          case "CANCELLED":
            return "已取消";
          case "COMPLETED":
            return "已完成";
          case "FAILED":
            return "失败";
          case "RUNNING":
            return "运行中";
          case "STARTING":
            return "正在启动";
          default:
            return state || "未知";
        }
      },
      workerSessionIDLabel: "Worker 会话 ID",
      sourceErrorHeading: "无法加载 Worker 会话记录",
      sourceLabel: "来源",
      sourceSequenceLabel: (sequence) => `来源序列 ${sequence}`,
      sessionTargetEmpty: "没有与此工作关联的 Worker 会话尝试。",
      sessionTargetError: "无法加载 Worker 会话尝试。",
      sessionTargetLoading: "正在加载 Worker 会话尝试…",
      sessionTargetListLabel: "Worker 会话尝试",
      sessionTargetRetry: "重试 Worker 会话查找",
      successorLabel: "后继 Worker 会话",
      terminalOutcomeLabel: (outcome) =>
        outcome === "SUCCESS"
          ? "成功"
          : outcome === "FAILURE"
            ? "失败"
            : "已取消",
      terminalOutcomeHeading: "终端结果",
      windowNavigationLabel: "时间线窗口导航",
      windowRangeLabel: (start, end, total) =>
        `事件 ${start}–${end}，共 ${total} 个`,
      timelineTitle: "Worker 会话时间线",
      toolArgumentsLabel: "参数",
      toolCallIDLabel: "工具调用 ID",
      toolLabel: "工具",
      toolNameLabel: "工具名称",
      toolOutputLabel: "工具输出",
      toolResultLabel: "结果",
      toolStatusLabel: "工具状态",
      totalTokensLabel: "令牌总数",
      turnLabel: "轮次",
      unknownValueLabel: "未知",
      unknownCategoryLabel: "规范事件",
      usageLabel: "用量",
      usageModelLabel: "用量模型",
      durationLabel: "时长",
      durationBasisLabel: "时长依据",
      durationValue: (durationMillis) =>
        durationMillis === null ? "未知" : `${durationMillis} 毫秒`,
      failureSummaryLabel: "失败摘要",
      modelUnavailable: "模型不可用",
      workIDsLabel: "工作 ID",
      workLabel: "工作",
      workSelectionRequired: "选择一个工作项以查看其 Worker 会话时间线。",
      categoryLabel: {
        error: "错误",
        "file-change": "文件变更",
        generic: "规范事件",
        message: "消息",
        plan: "计划",
        progress: "进度",
        reasoning: "推理",
        run: "运行",
        session: "会话",
        "stream-gap": "流间隙",
        tool: "工具",
        turn: "轮次",
        usage: "用量",
      },
      phaseLabel: (phase) => phase,
    },
  };

export function getWorkerSessionTimelineMessages(
  locale?: string | null,
): WorkerSessionTimelineMessages {
  return resolveLocalizedMessages(
    workerSessionTimelineMessagesByLocale,
    locale,
  );
}

export { workerSessionTimelineMessagesByLocale };
