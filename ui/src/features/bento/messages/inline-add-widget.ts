import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface InlineAddWidgetMessages {
  badge: string;
  body: string;
  hint: string;
  iconTitle: string;
  title: string;
}

const inlineAddWidgetMessagesByLocale = {
  en: {
    badge: "Dashboard",
    body: "Add a widget to this dashboard grid.",
    hint: "The widget picker opens here in the next step.",
    iconTitle: "Add widget icon",
    title: "Add widget",
  },
  "zh-CN": {
    badge: "仪表板",
    body: "将小组件添加到此仪表板网格。",
    hint: "下一步会在这里打开小组件选择器。",
    iconTitle: "添加小组件图标",
    title: "添加小组件",
  },
} satisfies LocalizedMessageCatalog<InlineAddWidgetMessages>;

export function getInlineAddWidgetMessages(
  locale?: string | null,
): InlineAddWidgetMessages {
  return resolveLocalizedMessages(inlineAddWidgetMessagesByLocale, locale);
}

export { inlineAddWidgetMessagesByLocale };
