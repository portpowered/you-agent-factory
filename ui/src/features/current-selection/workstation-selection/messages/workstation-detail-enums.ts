import {
  localizeEnumLabel,
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { ApiWorkstationKind, ApiWorkstationType } from "./workstation-openapi-enums";
import {
  localizeRunnerSelectionSourceValue,
  type ApiRunnerSelectionSource,
} from "./runner-openapi-enums";
import {
  localizeWorkstationKindValue,
  localizeWorkstationTypeValue,
} from "./workstation-openapi-enums";

export interface WorkstationDetailEnumMessages {
  localizeProviderSessionKind: (value: string) => string;
  localizeRunnerSelectionSource: (
    value: ApiRunnerSelectionSource | string,
  ) => string;
  localizeWorkstationBehavior: (value: ApiWorkstationKind | string) => string;
  localizeWorkstationKind: (value: ApiWorkstationKind | string) => string;
  localizeWorkstationType: (value: ApiWorkstationType | string) => string;
}

const workstationDetailEnumMessagesByLocale = {
  en: {
    localizeRunnerSelectionSource: (value) =>
      localizeRunnerSelectionSourceValue(value, "en"),
    localizeProviderSessionKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          path: "Path",
          session_id: "Session ID",
        },
        locale: "en",
        value,
      }),
    localizeWorkstationBehavior: (value) =>
      localizeWorkstationKindValue(value, "en"),
    localizeWorkstationKind: (value) =>
      localizeWorkstationKindValue(value, "en"),
    localizeWorkstationType: (value) =>
      localizeWorkstationTypeValue(value, "en"),
  },
  ja: {
    localizeRunnerSelectionSource: (value) =>
      localizeRunnerSelectionSourceValue(value, "ja"),
    localizeProviderSessionKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          path: "パス",
          session_id: "セッション ID",
        },
        locale: "ja",
        value,
      }),
    localizeWorkstationBehavior: (value) =>
      localizeWorkstationKindValue(value, "ja"),
    localizeWorkstationKind: (value) =>
      localizeWorkstationKindValue(value, "ja"),
    localizeWorkstationType: (value) =>
      localizeWorkstationTypeValue(value, "ja"),
  },
  ko: {
    localizeRunnerSelectionSource: (value) =>
      localizeRunnerSelectionSourceValue(value, "ko"),
    localizeProviderSessionKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          path: "경로",
          session_id: "세션 ID",
        },
        locale: "ko",
        value,
      }),
    localizeWorkstationBehavior: (value) =>
      localizeWorkstationKindValue(value, "ko"),
    localizeWorkstationKind: (value) =>
      localizeWorkstationKindValue(value, "ko"),
    localizeWorkstationType: (value) =>
      localizeWorkstationTypeValue(value, "ko"),
  },
  "zh-CN": {
    localizeRunnerSelectionSource: (value) =>
      localizeRunnerSelectionSourceValue(value, "zh-CN"),
    localizeProviderSessionKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          path: "路径",
          session_id: "会话 ID",
        },
        locale: "zh-CN",
        value,
      }),
    localizeWorkstationBehavior: (value) =>
      localizeWorkstationKindValue(value, "zh-CN"),
    localizeWorkstationKind: (value) =>
      localizeWorkstationKindValue(value, "zh-CN"),
    localizeWorkstationType: (value) =>
      localizeWorkstationTypeValue(value, "zh-CN"),
  },
} satisfies LocalizedMessageCatalog<WorkstationDetailEnumMessages>;

export function getWorkstationDetailEnumMessages(
  locale?: string | null,
): WorkstationDetailEnumMessages {
  return resolveLocalizedMessages(workstationDetailEnumMessagesByLocale, locale);
}
