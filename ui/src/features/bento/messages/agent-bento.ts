import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface AgentBentoMessages {
  boardLabel: string;
}

const agentBentoMessagesByLocale = {
  en: {
    boardLabel: "Infinite You bento board",
  },
  "zh-CN": {
    boardLabel: "Infinite You Bento 看板",
  },
} satisfies LocalizedMessageCatalog<AgentBentoMessages>;

export function getAgentBentoMessages(
  locale?: string | null,
): AgentBentoMessages {
  return resolveLocalizedMessages(agentBentoMessagesByLocale, locale);
}

export { agentBentoMessagesByLocale };
