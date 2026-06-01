import {
  localizeEnumLabel,
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { WorkstationLevelGuardType } from "../../../current-factory-definition/lib/workstation-guards";
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
  localizeWorkstationGuardType: (value: WorkstationLevelGuardType | string) => string;
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
    localizeWorkstationGuardType: (value) =>
      localizeWorkstationGuardTypeValue(value, "en"),
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
    localizeWorkstationGuardType: (value) =>
      localizeWorkstationGuardTypeValue(value, "ja"),
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
    localizeWorkstationGuardType: (value) =>
      localizeWorkstationGuardTypeValue(value, "ko"),
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
    localizeWorkstationGuardType: (value) =>
      localizeWorkstationGuardTypeValue(value, "zh-CN"),
  },
} satisfies LocalizedMessageCatalog<WorkstationDetailEnumMessages>;

function localizeWorkstationGuardTypeValue(
  value: WorkstationLevelGuardType | string,
  locale: "en" | "ja" | "ko" | "zh-CN",
): string {
  return localizeEnumLabel({
    category: "type",
    labels:
      locale === "ja"
        ? {
            MATCHES_FIELDS: "フィールド一致",
            VISIT_COUNT: "訪問回数",
          }
        : locale === "ko"
          ? {
              MATCHES_FIELDS: "필드 일치",
              VISIT_COUNT: "방문 횟수",
            }
          : locale === "zh-CN"
            ? {
                MATCHES_FIELDS: "字段匹配",
                VISIT_COUNT: "访问次数",
              }
            : {
                MATCHES_FIELDS: "Matches fields",
                VISIT_COUNT: "Visit count",
              },
    locale,
    value,
  });
}

export function getWorkstationDetailEnumMessages(
  locale?: string | null,
): WorkstationDetailEnumMessages {
  return resolveLocalizedMessages(workstationDetailEnumMessagesByLocale, locale);
}
