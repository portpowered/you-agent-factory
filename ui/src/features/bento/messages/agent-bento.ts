import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface AgentBentoMessages {
  boardLabel: string;
  cardErrorDescription: string;
  cardErrorTitle: string;
  layoutChanged: (
    widgetTitle: string,
    layout: { h: number; w: number; x: number; y: number },
  ) => string;
  layoutEditCancelled: (widgetTitle: string) => string;
  layoutEditStarted: (widgetTitle: string, mode: "move" | "resize") => string;
  layoutBoundary: (widgetTitle: string, mode: "move" | "resize") => string;
  moveInstructions: string;
  moveWidgetLabel: (widgetTitle: string) => string;
  removeWidgetLabel: (widgetTitle: string) => string;
  retryCard: string;
  resizeInstructions: string;
  resizeWidgetLabel: (widgetTitle: string) => string;
}

const agentBentoMessagesByLocale = {
  en: {
    boardLabel: "you-agent-factory bento board",
    cardErrorDescription:
      "This dashboard card could not be rendered. Retry it without affecting the rest of your dashboard.",
    cardErrorTitle: "Dashboard card unavailable",
    layoutBoundary: (widgetTitle: string, mode: "move" | "resize") =>
      `No ${mode} change was possible for ${widgetTitle}; the card is at a boundary or would overlap another card.`,
    layoutChanged: (
      widgetTitle: string,
      layout: { h: number; w: number; x: number; y: number },
    ) =>
      `${widgetTitle} is at column ${layout.x + 1}, row ${layout.y + 1}, ${layout.w} columns by ${layout.h} rows.`,
    layoutEditCancelled: (widgetTitle: string) =>
      `Layout editing cancelled for ${widgetTitle}.`,
    layoutEditStarted: (widgetTitle: string, mode: "move" | "resize") =>
      `Layout editing started for ${widgetTitle} in ${mode} mode.`,
    moveInstructions:
      "Move mode: use arrow keys to move one grid cell at a time. Press Enter or Space to save, or Escape to cancel.",
    moveWidgetLabel: (widgetTitle: string) => `Move ${widgetTitle} card`,
    removeWidgetLabel: (widgetTitle: string) =>
      `Remove ${widgetTitle} widget from dashboard`,
    retryCard: "Retry card",
    resizeInstructions:
      "Resize mode: use Left and Up to shrink, or Right and Down to grow by one grid cell. Press Enter or Space to save, or Escape to cancel.",
    resizeWidgetLabel: (widgetTitle: string) => `Resize ${widgetTitle} card`,
  },
  "zh-CN": {
    boardLabel: "you-agent-factory Bento 看板",
    cardErrorDescription:
      "此仪表板卡片无法渲染。重试此卡片不会影响仪表板的其余部分。",
    cardErrorTitle: "仪表板卡片不可用",
    layoutBoundary: (widgetTitle: string, mode: "move" | "resize") =>
      `${widgetTitle} 无法继续${mode === "move" ? "移动" : "调整大小"}；卡片已到边界或会与其他卡片重叠。`,
    layoutChanged: (
      widgetTitle: string,
      layout: { h: number; w: number; x: number; y: number },
    ) =>
      `${widgetTitle} 位于第 ${layout.x + 1} 列、第 ${layout.y + 1} 行，大小为 ${layout.w} 列乘 ${layout.h} 行。`,
    layoutEditCancelled: (widgetTitle: string) =>
      `已取消编辑 ${widgetTitle} 的布局。`,
    layoutEditStarted: (widgetTitle: string, mode: "move" | "resize") =>
      `已开始在${mode === "move" ? "移动" : "调整大小"}模式下编辑 ${widgetTitle} 的布局。`,
    moveInstructions:
      "移动模式：使用方向键每次移动一个网格单元。按 Enter 或空格保存，按 Escape 取消。",
    moveWidgetLabel: (widgetTitle: string) => `移动 ${widgetTitle} 卡片`,
    removeWidgetLabel: (widgetTitle: string) =>
      `从仪表板移除 ${widgetTitle} 小组件`,
    retryCard: "重试卡片",
    resizeInstructions:
      "调整大小模式：使用左键和上键缩小，或使用右键和下键按一个网格单元放大。按 Enter 或空格保存，按 Escape 取消。",
    resizeWidgetLabel: (widgetTitle: string) => `调整 ${widgetTitle} 卡片大小`,
  },
} satisfies LocalizedMessageCatalog<AgentBentoMessages>;

export function getAgentBentoMessages(
  locale?: string | null,
): AgentBentoMessages {
  return resolveLocalizedMessages(agentBentoMessagesByLocale, locale);
}

export { agentBentoMessagesByLocale };
