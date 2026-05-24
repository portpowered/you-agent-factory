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
      "セッション詳細を確認するには、作業項目またはワークステーション履歴からプロバイダー セッションを選択してください。",
    title: "プロバイダー セッション",
  },
  ko: {
    emptyState:
      "세션 세부 정보를 확인하려면 작업 항목 또는 워크스테이션 기록에서 공급자 세션을 선택하세요.",
    title: "공급자 세션",
  },
  "zh-CN": {
    emptyState:
      "从工作项或工作站历史中选择一个提供方会话以查看会话详情。",
    title: "提供方会话",
  },
} satisfies LocalizedMessages<ProviderSessionWidgetMessages>;

export function getProviderSessionWidgetMessages(locale?: string | null) {
  return resolveLocalizedMessages(providerSessionWidgetMessagesByLocale, locale);
}
