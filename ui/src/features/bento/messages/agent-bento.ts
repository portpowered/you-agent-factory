import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface AgentBentoMessages {
  boardLabel: string;
  cardErrorDescription: string;
  cardErrorTitle: string;
  removeWidgetLabel: (widgetTitle: string) => string;
  retryCard: string;
}

const agentBentoMessagesByLocale = {
  en: {
    boardLabel: "you-agent-factory bento board",
    cardErrorDescription:
      "This dashboard card could not be rendered. Retry it without affecting the rest of your dashboard.",
    cardErrorTitle: "Dashboard card unavailable",
    removeWidgetLabel: (widgetTitle: string) =>
      `Remove ${widgetTitle} widget from dashboard`,
    retryCard: "Retry card",
  },
  "zh-CN": {
    boardLabel: "you-agent-factory Bento 看板",
    cardErrorDescription:
      "此仪表板卡片无法渲染。重试此卡片不会影响仪表板的其余部分。",
    cardErrorTitle: "仪表板卡片不可用",
    removeWidgetLabel: (widgetTitle: string) =>
      `从仪表板移除 ${widgetTitle} 小组件`,
    retryCard: "重试卡片",
  },
} satisfies LocalizedMessageCatalog<AgentBentoMessages>;

export function getAgentBentoMessages(
  locale?: string | null,
): AgentBentoMessages {
  return resolveLocalizedMessages(agentBentoMessagesByLocale, locale);
}

export { agentBentoMessagesByLocale };
