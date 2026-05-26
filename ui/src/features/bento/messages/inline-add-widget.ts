import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface InlineAddWidgetMessages {
  actionLabel: string;
  actionUnavailableLabel: string;
  badge: string;
  body: string;
  iconTitle: string;
  pickerOpenState: string;
  readyState: string;
  title: string;
  unavailableHint: string;
  unavailableState: string;
}

const inlineAddWidgetMessagesByLocale = {
  en: {
    actionLabel: "Browse widgets",
    actionUnavailableLabel: "No widgets available",
    badge: "Dashboard",
    body: "Add another dashboard card from this inline slot.",
    iconTitle: "Add widget icon",
    pickerOpenState: "Widget picker open",
    readyState: "Ready to add",
    title: "Add widget",
    unavailableHint: "Remove a singleton widget or keep using duplicate-capable widgets to make room for a different card.",
    unavailableState: "No additional widgets are available from this layout.",
  },
  "zh-CN": {
    actionLabel: "浏览小组件",
    actionUnavailableLabel: "没有可用小组件",
    badge: "仪表板",
    body: "从这个内联位置添加另一个仪表板卡片。",
    iconTitle: "添加小组件图标",
    pickerOpenState: "小组件选择器已打开",
    readyState: "可添加",
    title: "添加小组件",
    unavailableHint: "移除单例小组件，或继续使用可重复的小组件，以便为其他卡片腾出空间。",
    unavailableState: "当前布局中没有更多可添加的小组件。",
  },
} satisfies LocalizedMessageCatalog<InlineAddWidgetMessages>;

export function getInlineAddWidgetMessages(
  locale?: string | null,
): InlineAddWidgetMessages {
  return resolveLocalizedMessages(inlineAddWidgetMessagesByLocale, locale);
}

export { inlineAddWidgetMessagesByLocale };
