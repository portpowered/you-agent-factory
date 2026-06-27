export interface FactorySessionLifecycleOutcomeMessages {
  lifecycleOutcomeAcceptedLabel: string;
  lifecycleOutcomeAcceptedTitle: (actionLabel: string) => string;
  lifecycleOutcomeConflictLabel: string;
  lifecycleOutcomeConflictTitle: (actionLabel: string) => string;
  lifecycleOutcomeCurrentStatusDetail: (statusLabel: string) => string;
  lifecycleOutcomeInvalidStateLabel: string;
  lifecycleOutcomeInvalidStateTitle: (actionLabel: string) => string;
  lifecycleOutcomeNoOpLabel: string;
  lifecycleOutcomeNoOpTitle: (actionLabel: string) => string;
  lifecycleOutcomeRetryDispatchDetail: (dispatchID: string) => string;
  lifecycleOutcomeTerminalSessionLabel: string;
  lifecycleOutcomeTerminalSessionTitle: (actionLabel: string) => string;
  lifecycleOutcomeTransportErrorLabel: string;
  lifecycleOutcomeTransportErrorTitle: (actionLabel: string) => string;
}

export const englishFactorySessionLifecycleOutcomeMessages = {
  lifecycleOutcomeAcceptedLabel: "Accepted",
  lifecycleOutcomeAcceptedTitle: (actionLabel) => `${actionLabel} accepted`,
  lifecycleOutcomeConflictLabel: "Conflict",
  lifecycleOutcomeConflictTitle: (actionLabel) =>
    `${actionLabel} is blocked by another lifecycle change.`,
  lifecycleOutcomeCurrentStatusDetail: (statusLabel) =>
    `Current durable status: ${statusLabel}.`,
  lifecycleOutcomeInvalidStateLabel: "Invalid state",
  lifecycleOutcomeInvalidStateTitle: (actionLabel) =>
    `${actionLabel} is not available in the current session state.`,
  lifecycleOutcomeNoOpLabel: "No-op",
  lifecycleOutcomeNoOpTitle: (actionLabel) =>
    `${actionLabel} was already satisfied.`,
  lifecycleOutcomeRetryDispatchDetail: (dispatchID) =>
    `Retry dispatch: ${dispatchID}.`,
  lifecycleOutcomeTerminalSessionLabel: "Terminal session",
  lifecycleOutcomeTerminalSessionTitle: (actionLabel) =>
    `${actionLabel} is unavailable because this Factory Session is already terminal.`,
  lifecycleOutcomeTransportErrorLabel: "Request failed",
  lifecycleOutcomeTransportErrorTitle: (actionLabel) =>
    `${actionLabel} could not be submitted.`,
} satisfies FactorySessionLifecycleOutcomeMessages;

export const chineseFactorySessionLifecycleOutcomeMessages = {
  lifecycleOutcomeAcceptedLabel: "已接受",
  lifecycleOutcomeAcceptedTitle: (actionLabel) => `已接受“${actionLabel}”请求`,
  lifecycleOutcomeConflictLabel: "冲突",
  lifecycleOutcomeConflictTitle: (actionLabel) =>
    `“${actionLabel}”被另一个生命周期变更阻塞。`,
  lifecycleOutcomeCurrentStatusDetail: (statusLabel) =>
    `当前持久化状态：${statusLabel}。`,
  lifecycleOutcomeInvalidStateLabel: "状态无效",
  lifecycleOutcomeInvalidStateTitle: (actionLabel) =>
    `当前会话状态不允许执行“${actionLabel}”。`,
  lifecycleOutcomeNoOpLabel: "无变化",
  lifecycleOutcomeNoOpTitle: (actionLabel) =>
    `“${actionLabel}”已经满足，无需再次执行。`,
  lifecycleOutcomeRetryDispatchDetail: (dispatchID) =>
    `重试调度：${dispatchID}。`,
  lifecycleOutcomeTerminalSessionLabel: "终态会话",
  lifecycleOutcomeTerminalSessionTitle: (actionLabel) =>
    `此工厂会话已进入终态，无法执行“${actionLabel}”。`,
  lifecycleOutcomeTransportErrorLabel: "请求失败",
  lifecycleOutcomeTransportErrorTitle: (actionLabel) =>
    `无法提交“${actionLabel}”请求。`,
} satisfies FactorySessionLifecycleOutcomeMessages;
