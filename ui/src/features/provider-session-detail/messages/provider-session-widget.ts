import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface ProviderSessionWidgetMessages {
  emptyState: string;
  title: string;
}

const providerSessionWidgetMessagesByLocale = {
  en: {
    emptyState:
      "Select a provider session from work-item or workstation history to inspect session details.",
    title: "Provider session",
  },
  ja: {
    emptyState:
      "セッション詳細を確認するには、作業項目またはワークステーション履歴から provider session を選択してください。",
    title: "Provider session",
  },
  ko: {
    emptyState:
      "세션 세부 정보를 확인하려면 작업 항목 또는 워크스테이션 기록에서 provider session을 선택하세요.",
    title: "Provider session",
  },
  "zh-CN": {
    emptyState:
      "从工作项或工作站历史中选择一个 provider session 以查看会话详情。",
    title: "Provider session",
  },
} satisfies LocalizedMessages<ProviderSessionWidgetMessages>;

export function getProviderSessionWidgetMessages(locale?: string | null) {
  return resolveLocalizedMessages(providerSessionWidgetMessagesByLocale, locale);
}
