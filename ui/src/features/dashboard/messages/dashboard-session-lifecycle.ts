import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardSessionLifecycleMessages {
  artifactRefsLabel: string;
  completedLabel: string;
  failureLabel: string;
  lifecycleControlStatusLabel: string;
  partialResultLabel: string;
  phaseLabel: string;
  recoveryFailedStreamLabel: string;
  reconnectingStreamLabel: string;
  resultStatusLabel: string;
  sessionPausedLabel: string;
  sessionRunningLabel: string;
  sessionStartedLabel: string;
  staleStreamLabel: string;
  terminalSuccessLabel: string;
}

const dashboardSessionLifecycleMessagesByLocale = {
  en: {
    artifactRefsLabel: "Artifact refs",
    completedLabel: "Session completed",
    failureLabel: "Session failed",
    lifecycleControlStatusLabel: "Factory Session lifecycle",
    partialResultLabel: "Partial result available",
    phaseLabel: "Current phase",
    recoveryFailedStreamLabel:
      "The dashboard could not restore this session automatically.",
    reconnectingStreamLabel: "Reconnecting event stream",
    resultStatusLabel: "Result status",
    sessionPausedLabel: "Factory Session paused",
    sessionRunningLabel: "Factory Session running",
    sessionStartedLabel: "Session started",
    staleStreamLabel: "Event stream stale",
    terminalSuccessLabel: "Session finished",
  },
  "zh-CN": {
    artifactRefsLabel: "工件引用",
    completedLabel: "会话已完成",
    failureLabel: "会话失败",
    lifecycleControlStatusLabel: "工厂会话生命周期",
    partialResultLabel: "部分结果可用",
    phaseLabel: "当前阶段",
    recoveryFailedStreamLabel: "仪表板无法自动恢复此会话。",
    reconnectingStreamLabel: "正在重新连接事件流",
    resultStatusLabel: "结果状态",
    sessionPausedLabel: "工厂会话已暂停",
    sessionRunningLabel: "工厂会话运行中",
    sessionStartedLabel: "会话已开始",
    staleStreamLabel: "事件流已过期",
    terminalSuccessLabel: "会话已结束",
  },
} satisfies LocalizedMessageCatalog<DashboardSessionLifecycleMessages>;

export function getDashboardSessionLifecycleMessages(
  locale?: string | null,
): DashboardSessionLifecycleMessages {
  return resolveLocalizedMessages(
    dashboardSessionLifecycleMessagesByLocale,
    locale,
  );
}
