import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface InlineAddWidgetMessages {
  actionLabel: string;
  actionUnavailableLabel: string;
  title: string;
}

const inlineAddWidgetMessagesByLocale = {
  en: {
    actionLabel: "Browse widgets",
    actionUnavailableLabel: "No widgets available",
    title: "Add widget",
  },
  "zh-CN": {
    actionLabel: "浏览小组件",
    actionUnavailableLabel: "没有可用小组件",
    title: "添加小组件",
  },
} satisfies LocalizedMessageCatalog<InlineAddWidgetMessages>;

export function getInlineAddWidgetMessages(
  locale?: string | null,
): InlineAddWidgetMessages {
  return resolveLocalizedMessages(inlineAddWidgetMessagesByLocale, locale);
}

export { inlineAddWidgetMessagesByLocale };
