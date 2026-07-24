import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactorySessionWidgetMessages {
  emptyState: string;
  title: string;
}

const factorySessionWidgetMessagesByLocale = {
  en: {
    emptyState:
      "Select a live factory session to inspect orchestrator runtime.",
    title: "Factory session",
  },
  "zh-CN": {
    emptyState: "选择一个实时工厂会话来查看编排器运行时。",
    title: "工厂会话",
  },
} satisfies LocalizedMessageCatalog<FactorySessionWidgetMessages>;

export function getFactorySessionWidgetMessages(
  locale?: string | null,
): FactorySessionWidgetMessages {
  return resolveLocalizedMessages(factorySessionWidgetMessagesByLocale, locale);
}
