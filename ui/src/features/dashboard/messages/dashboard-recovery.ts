import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardRecoveryMessages {
  recoveryFailedDetail: string;
  recoveryFailedRefreshLabel: string;
  recoveryFailedRetryLabel: string;
  recoveryFailedTitle: string;
}

const dashboardRecoveryMessagesByLocale = {
  en: {
    recoveryFailedDetail:
      "The dashboard could not restore this session automatically after the event cursor expired. Retry the session stream or refresh the page.",
    recoveryFailedRefreshLabel: "Refresh page",
    recoveryFailedRetryLabel: "Retry session stream",
    recoveryFailedTitle: "Session replay needs attention",
  },
  ja: {
    recoveryFailedDetail:
      "イベントカーソルの期限切れ後に、このセッションをダッシュボードで自動復元できませんでした。セッションストリームを再試行するか、ページを再読み込みしてください。",
    recoveryFailedRefreshLabel: "ページを再読み込み",
    recoveryFailedRetryLabel: "セッションストリームを再試行",
    recoveryFailedTitle: "セッションの再生を復旧できません",
  },
  ko: {
    recoveryFailedDetail:
      "이벤트 커서가 만료된 뒤 이 세션을 대시보드에서 자동으로 복원하지 못했습니다. 세션 스트림을 다시 시도하거나 페이지를 새로고침하세요.",
    recoveryFailedRefreshLabel: "페이지 새로고침",
    recoveryFailedRetryLabel: "세션 스트림 다시 시도",
    recoveryFailedTitle: "세션 재생을 복구할 수 없음",
  },
  "zh-CN": {
    recoveryFailedDetail:
      "事件游标过期后，仪表板无法自动恢复此会话。请重试会话事件流，或刷新页面。",
    recoveryFailedRefreshLabel: "刷新页面",
    recoveryFailedRetryLabel: "重试会话事件流",
    recoveryFailedTitle: "会话重放需要处理",
  },
} satisfies LocalizedMessageCatalog<DashboardRecoveryMessages>;

export function getDashboardRecoveryMessages(
  locale?: string | null,
): DashboardRecoveryMessages {
  return resolveLocalizedMessages(dashboardRecoveryMessagesByLocale, locale);
}
