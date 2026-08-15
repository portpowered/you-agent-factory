import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardCardStateMessages {
  historicalLabel: string;
  knownEmptyLabel: string;
  liveLabel: string;
  loadingLabel: string;
  recoveringLabel: string;
  reconnectingLabel: string;
  staleLabel: string;
  unavailableLabel: string;
  statusAriaLabel: (state: string, temporal: string) => string;
}

const dashboardCardStateMessagesByLocale = {
  en: {
    historicalLabel: "Historical",
    knownEmptyLabel: "No data yet",
    liveLabel: "Live",
    loadingLabel: "Loading",
    recoveringLabel: "Recovering",
    reconnectingLabel: "Reconnecting",
    staleLabel: "Stale data",
    unavailableLabel: "Data unavailable",
    statusAriaLabel: (state, temporal) => `Card state: ${state}. ${temporal}.`,
  },
  "zh-CN": {
    historicalLabel: "历史",
    knownEmptyLabel: "暂无数据",
    liveLabel: "实时",
    loadingLabel: "加载中",
    recoveringLabel: "恢复中",
    reconnectingLabel: "重新连接中",
    staleLabel: "数据已过期",
    unavailableLabel: "数据不可用",
    statusAriaLabel: (state, temporal) => `卡片状态：${state}。${temporal}。`,
  },
} satisfies LocalizedMessageCatalog<DashboardCardStateMessages>;

export function getDashboardCardStateMessages(
  locale?: string | null,
): DashboardCardStateMessages {
  return resolveLocalizedMessages(dashboardCardStateMessagesByLocale, locale);
}
