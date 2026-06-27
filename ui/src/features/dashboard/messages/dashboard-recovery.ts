import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardRecoveryMessages {
  preflightRetryAction: string;
  sessionNotFoundDetailTemplate: string;
  sessionNotFoundTitle: string;
  unknownRecoveryDetail: string;
  unknownRecoveryTitle: string;
}

const dashboardRecoveryMessagesByLocale = {
  en: {
    preflightRetryAction: "Retry clean replay",
    sessionNotFoundDetailTemplate:
      'The backend could not resolve the live session for "{{sessionId}}". Cached dashboard state was cleared. Reopen the session, then retry a clean replay.',
    sessionNotFoundTitle: "Session recovery required",
    unknownRecoveryDetail:
      "The backend rejected the cached dashboard recovery state. Cached replay data was cleared. Retry a clean replay when the session is ready.",
    unknownRecoveryTitle: "Dashboard recovery blocked",
  },
  "zh-CN": {
    preflightRetryAction: "重试干净回放",
    sessionNotFoundDetailTemplate:
      '后端无法解析“{{sessionId}}”的活动会话。缓存的仪表板状态已清除。重新打开该会话后，再重试一次干净回放。',
    sessionNotFoundTitle: "需要恢复会话",
    unknownRecoveryDetail:
      "后端拒绝了缓存的仪表板恢复状态。缓存的回放数据已清除。会话准备好后，请重试干净回放。",
    unknownRecoveryTitle: "仪表板恢复被阻止",
  },
} satisfies LocalizedMessageCatalog<DashboardRecoveryMessages>;

export function getDashboardRecoveryMessages(
  locale?: string | null,
): DashboardRecoveryMessages {
  return resolveLocalizedMessages(dashboardRecoveryMessagesByLocale, locale);
}
