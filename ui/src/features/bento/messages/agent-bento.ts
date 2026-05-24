import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface AgentBentoMessages {
  boardLabel: string;
  removeWidgetLabel: (widgetTitle: string) => string;
}

const agentBentoMessagesByLocale = {
  en: {
    boardLabel: "you-agent-factory bento board",
    removeWidgetLabel: (widgetTitle: string) =>
      `Remove ${widgetTitle} widget from dashboard`,
  },
  "zh-CN": {
    boardLabel: "you-agent-factory Bento 看板",
    removeWidgetLabel: (widgetTitle: string) =>
      `从仪表板移除 ${widgetTitle} 小组件`,
  },
} satisfies LocalizedMessageCatalog<AgentBentoMessages>;

export function getAgentBentoMessages(
  locale?: string | null,
): AgentBentoMessages {
  return resolveLocalizedMessages(agentBentoMessagesByLocale, locale);
}

export { agentBentoMessagesByLocale };
