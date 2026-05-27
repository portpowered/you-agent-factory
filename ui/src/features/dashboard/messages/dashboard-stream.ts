import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardStreamMessages {
  loadingFactoryEvents: string;
}

const dashboardStreamMessagesByLocale = {
  en: {
    loadingFactoryEvents: "Loading factory events...",
  },
  "zh-CN": {
    loadingFactoryEvents: "正在加载工厂事件...",
  },
} satisfies LocalizedMessageCatalog<DashboardStreamMessages>;

export function getDashboardStreamMessages(
  locale?: string | null,
): DashboardStreamMessages {
  return resolveLocalizedMessages(dashboardStreamMessagesByLocale, locale);
}

export { dashboardStreamMessagesByLocale };
