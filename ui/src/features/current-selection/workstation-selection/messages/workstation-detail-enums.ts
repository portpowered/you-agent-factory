import {
  type LocalizedMessageCatalog,
  localizeEnumLabel,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type {
  InputGuardType,
  WorkstationLevelGuardType,
} from "../../../current-factory-definition/lib/workstation-guards";
import {
  type ApiModelProviderSelectionSource,
  localizeRunnerSelectionSourceValue,
} from "./runner-openapi-enums";
import type {
  ApiWorkstationKind,
  ApiWorkstationType,
} from "./workstation-openapi-enums";
import {
  localizeWorkstationKindValue,
  localizeWorkstationTypeValue,
} from "./workstation-openapi-enums";

export interface WorkstationDetailEnumMessages {
  localizeProviderSessionKind: (value: string) => string;
  localizeRunnerSelectionSource: (
    value: ApiModelProviderSelectionSource | string,
  ) => string;
  localizeWorkstationBehavior: (value: ApiWorkstationKind | string) => string;
  localizeWorkstationKind: (value: ApiWorkstationKind | string) => string;
  localizeWorkstationType: (value: ApiWorkstationType | string) => string;
  localizeWorkstationGuardType: (
    value: WorkstationLevelGuardType | string,
  ) => string;
  localizeInputGuardType: (value: InputGuardType | string) => string;
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
    localizeInputGuardType: (value) => localizeInputGuardTypeValue(value, "en"),
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
    localizeInputGuardType: (value) => localizeInputGuardTypeValue(value, "ja"),
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
    localizeInputGuardType: (value) => localizeInputGuardTypeValue(value, "ko"),
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
    localizeInputGuardType: (value) =>
      localizeInputGuardTypeValue(value, "zh-CN"),
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

function localizeInputGuardTypeValue(
  value: InputGuardType | string,
  locale: "en" | "ja" | "ko" | "zh-CN",
): string {
  return localizeEnumLabel({
    category: "type",
    labels:
      locale === "ja"
        ? {
            ALL_CHILDREN_COMPLETE: "子完了待ち",
            ANY_CHILD_FAILED: "子失敗で失敗",
            SAME_NAME: "同名一致",
            SAME_TRACE_ID: "同一トレース ID",
          }
        : locale === "ko"
          ? {
              ALL_CHILDREN_COMPLETE: "자식 완료 대기",
              ANY_CHILD_FAILED: "자식 실패 시 실패",
              SAME_NAME: "동일 이름",
              SAME_TRACE_ID: "동일 추적 ID",
            }
          : locale === "zh-CN"
            ? {
                ALL_CHILDREN_COMPLETE: "等待子项完成",
                ANY_CHILD_FAILED: "任一子项失败",
                SAME_NAME: "同名匹配",
                SAME_TRACE_ID: "相同追踪 ID",
              }
            : {
                ALL_CHILDREN_COMPLETE: "All children complete",
                ANY_CHILD_FAILED: "Any child failed",
                SAME_NAME: "Same name",
                SAME_TRACE_ID: "Same trace ID",
              },
    locale,
    value,
  });
}

export function getWorkstationDetailEnumMessages(
  locale?: string | null,
): WorkstationDetailEnumMessages {
  return resolveLocalizedMessages(
    workstationDetailEnumMessagesByLocale,
    locale,
  );
}
