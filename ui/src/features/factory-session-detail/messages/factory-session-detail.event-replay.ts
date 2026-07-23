export interface FactorySessionEventReplayMessages {
  eventReplayArtifactCountLabel: (count: number) => string;
  eventReplayArtifactLabel: (artifactID: string) => string;
  eventReplayCheckpointRecordedTitle: string;
  eventReplayCheckpointLabel: (checkpointID: string) => string;
  eventReplayDispatchInterruptedTitle: string;
  eventReplayDispatchLabel: (dispatchID: string) => string;
  eventReplayDispatchQueuedTitle: string;
  eventReplayDispatchReconciledTitle: string;
  eventReplayDispatchStatusDetail: (status: string) => string;
  eventReplayEmptyState: string;
  eventReplayErrorState: string;
  eventReplayHeading: string;
  eventReplayHint: string;
  eventReplayLifecycleStatusDetail: (status: string) => string;
  eventReplayLoadingState: string;
  eventReplayNoContext: string;
  eventReplayPhaseChangedTitle: string;
  eventReplayPhaseLabel: (phase: string) => string;
  eventReplayQueuePositionLabel: (queuePosition: number) => string;
  eventReplayResultStatusDetail: (status: string) => string;
  eventReplayRetryPlannedLabel: string;
  eventReplaySequenceTickLabel: (sequence: number, tick: number) => string;
  eventReplaySessionCompletedTitle: string;
  eventReplaySessionResultUpdatedTitle: string;
  eventReplaySessionStartedTitle: string;
  eventReplaySequenceLabel: (sequence: number) => string;
  eventReplaySummary: (visibleCount: number, totalCount: number) => string;
  eventReplayUnavailableState: string;
  eventReplayVisibleSummary: (visibleCount: number) => string;
  eventReplayWarningCountLabel: (count: number) => string;
  eventReplayWorkLabel: (count: number) => string;
  expandEventReplayLabel: string;
  collapseEventReplayLabel: string;
}

export const englishFactorySessionEventReplayMessages = {
  eventReplayArtifactCountLabel: (count) =>
    `${count} Factory Artifact${count === 1 ? "" : "s"}`,
  eventReplayArtifactLabel: (artifactID) => `Factory Artifact ${artifactID}`,
  eventReplayCheckpointRecordedTitle: "Checkpoint recorded",
  eventReplayCheckpointLabel: (checkpointID) => `Checkpoint ${checkpointID}`,
  eventReplayDispatchInterruptedTitle: "Dispatch interrupted",
  eventReplayDispatchLabel: (dispatchID) => `Dispatch ${dispatchID}`,
  eventReplayDispatchQueuedTitle: "Dispatch queued",
  eventReplayDispatchReconciledTitle: "Dispatch reconciled",
  eventReplayDispatchStatusDetail: (status) =>
    `Dispatch status ${status.toLowerCase().replaceAll("_", " ")}`,
  eventReplayEmptyState:
    "No durable Factory Events are available for this session.",
  eventReplayErrorState: "The Factory Event replay could not be loaded.",
  eventReplayHeading: "Factory Event replay",
  eventReplayHint:
    "Reveal bounded durable Factory Event history without leaving this session detail view.",
  eventReplayLifecycleStatusDetail: (status) =>
    `Lifecycle status ${status.toLowerCase().replaceAll("_", " ")}`,
  eventReplayLoadingState: "Loading durable Factory Event replay…",
  eventReplayNoContext: "Session-level Factory Event",
  eventReplayPhaseChangedTitle: "Phase changed",
  eventReplayPhaseLabel: (phase) => `Phase ${phase}`,
  eventReplayQueuePositionLabel: (queuePosition) =>
    `Queue position ${queuePosition}`,
  eventReplayResultStatusDetail: (status) =>
    `Result status ${status.toLowerCase().replaceAll("_", " ")}`,
  eventReplayRetryPlannedLabel: "Retry planned",
  eventReplaySequenceTickLabel: (sequence, tick) =>
    `Session event ${sequence} · Tick ${tick}`,
  eventReplaySessionCompletedTitle: "Session completed",
  eventReplaySessionResultUpdatedTitle: "Session result updated",
  eventReplaySessionStartedTitle: "Session started",
  eventReplaySequenceLabel: (sequence) => `Session event ${sequence}`,
  eventReplaySummary: (visibleCount, totalCount) =>
    `Showing the latest ${visibleCount} of ${totalCount} Factory Events.`,
  eventReplayUnavailableState:
    "Durable Factory Event replay is unavailable for this session.",
  eventReplayVisibleSummary: (visibleCount) =>
    `Showing ${visibleCount} Factory Event${visibleCount === 1 ? "" : "s"}.`,
  eventReplayWarningCountLabel: (count) =>
    `${count} warning${count === 1 ? "" : "s"}`,
  eventReplayWorkLabel: (count) =>
    `${count} related work item${count === 1 ? "" : "s"}`,
  expandEventReplayLabel: "Expand Factory Event replay",
  collapseEventReplayLabel: "Collapse Factory Event replay",
} satisfies FactorySessionEventReplayMessages;

export const chineseFactorySessionEventReplayMessages = {
  eventReplayArtifactCountLabel: (count) => `${count} 个工厂工件`,
  eventReplayArtifactLabel: (artifactID) => `工厂工件 ${artifactID}`,
  eventReplayCheckpointRecordedTitle: "已记录检查点",
  eventReplayCheckpointLabel: (checkpointID) => `检查点 ${checkpointID}`,
  eventReplayDispatchInterruptedTitle: "调度已中断",
  eventReplayDispatchLabel: (dispatchID) => `调度 ${dispatchID}`,
  eventReplayDispatchQueuedTitle: "调度已排队",
  eventReplayDispatchReconciledTitle: "调度已对账",
  eventReplayDispatchStatusDetail: (status) =>
    `调度状态 ${status.toLowerCase().replaceAll("_", " ")}`,
  eventReplayEmptyState: "此会话没有可用的持久化工厂事件。",
  eventReplayErrorState: "无法加载工厂事件回放。",
  eventReplayHeading: "工厂事件回放",
  eventReplayHint: "在当前会话详情内展开受限的持久化工厂事件历史。",
  eventReplayLifecycleStatusDetail: (status) =>
    `生命周期状态 ${status.toLowerCase().replaceAll("_", " ")}`,
  eventReplayLoadingState: "正在加载持久化工厂事件回放…",
  eventReplayNoContext: "会话级工厂事件",
  eventReplayPhaseChangedTitle: "阶段已变更",
  eventReplayPhaseLabel: (phase) => `阶段 ${phase}`,
  eventReplayQueuePositionLabel: (queuePosition) => `队列位置 ${queuePosition}`,
  eventReplayResultStatusDetail: (status) =>
    `结果状态 ${status.toLowerCase().replaceAll("_", " ")}`,
  eventReplayRetryPlannedLabel: "计划重试",
  eventReplaySequenceTickLabel: (sequence, tick) =>
    `会话事件 ${sequence} · Tick ${tick}`,
  eventReplaySessionCompletedTitle: "会话已完成",
  eventReplaySessionResultUpdatedTitle: "会话结果已更新",
  eventReplaySessionStartedTitle: "会话已启动",
  eventReplaySequenceLabel: (sequence) => `会话事件 ${sequence}`,
  eventReplaySummary: (visibleCount, totalCount) =>
    `显示最新 ${visibleCount} / ${totalCount} 条工厂事件。`,
  eventReplayUnavailableState: "此会话的持久化工厂事件回放不可用。",
  eventReplayVisibleSummary: (visibleCount) =>
    `显示 ${visibleCount} 条工厂事件。`,
  eventReplayWarningCountLabel: (count) => `${count} 条警告`,
  eventReplayWorkLabel: (count) => `${count} 个关联工作项`,
  expandEventReplayLabel: "展开工厂事件回放",
  collapseEventReplayLabel: "收起工厂事件回放",
} satisfies FactorySessionEventReplayMessages;
