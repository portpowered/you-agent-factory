import type { components } from "../../../../api/generated/openapi";
import { type EnumLabelCatalog, localizeEnumLabel } from "../../../../i18n";

export type ApiWorkstationKind = components["schemas"]["WorkstationKind"];
export type ApiWorkstationType = components["schemas"]["WorkstationType"];

type WorkstationKindLabelCatalog = EnumLabelCatalog<ApiWorkstationKind>;
type WorkstationTypeLabelCatalog = EnumLabelCatalog<ApiWorkstationType>;

const WORKSTATION_KIND_LABELS_EN = {
  CRON: "Cron",
  POLLER: "Poller",
  REPEATER: "Repeater",
  STANDARD: "Standard",
} satisfies WorkstationKindLabelCatalog["labels"];

const WORKSTATION_KIND_LABELS_JA = {
  CRON: "Cron",
  POLLER: "ポーラー",
  REPEATER: "リピーター",
  STANDARD: "標準",
} satisfies WorkstationKindLabelCatalog["labels"];

const WORKSTATION_KIND_LABELS_KO = {
  CRON: "Cron",
  POLLER: "폴러",
  REPEATER: "반복기",
  STANDARD: "표준",
} satisfies WorkstationKindLabelCatalog["labels"];

const WORKSTATION_KIND_LABELS_ZH_CN = {
  CRON: "Cron",
  POLLER: "轮询器",
  REPEATER: "重复器",
  STANDARD: "标准",
} satisfies WorkstationKindLabelCatalog["labels"];

const WORKSTATION_TYPE_LABELS_EN = {
  AGENT_RUN: "Agent run",
  CLASSIFIER_WORKSTATION: "Classifier workstation",
  HUMAN_APPROVAL: "Human approval",
  INFERENCE_RUN: "Inference run",
  LOGICAL_MOVE: "Logical move",
  MODEL_INVOKE: "Model invoke (legacy)",
  MODEL_WORKSTATION: "Model workstation (legacy)",
  POLLER_RUN: "Poller run",
  SCRIPT_RUN: "Script run",
} satisfies WorkstationTypeLabelCatalog["labels"];

const WORKSTATION_TYPE_LABELS_JA = {
  AGENT_RUN: "エージェント実行",
  CLASSIFIER_WORKSTATION: "分類ワークステーション",
  HUMAN_APPROVAL: "人による承認",
  INFERENCE_RUN: "推論実行",
  LOGICAL_MOVE: "論理移動",
  MODEL_INVOKE: "モデル呼び出し（レガシー）",
  MODEL_WORKSTATION: "モデルワークステーション（レガシー）",
  POLLER_RUN: "ポーラー実行",
  SCRIPT_RUN: "スクリプト実行",
} satisfies WorkstationTypeLabelCatalog["labels"];

const WORKSTATION_TYPE_LABELS_KO = {
  AGENT_RUN: "에이전트 실행",
  CLASSIFIER_WORKSTATION: "분류 워크스테이션",
  HUMAN_APPROVAL: "사람 승인",
  INFERENCE_RUN: "추론 실행",
  LOGICAL_MOVE: "논리 이동",
  MODEL_INVOKE: "모델 호출(레거시)",
  MODEL_WORKSTATION: "모델 워크스테이션(레거시)",
  POLLER_RUN: "폴러 실행",
  SCRIPT_RUN: "스크립트 실행",
} satisfies WorkstationTypeLabelCatalog["labels"];

const WORKSTATION_TYPE_LABELS_ZH_CN = {
  AGENT_RUN: "Agent 运行",
  CLASSIFIER_WORKSTATION: "分类工作站",
  HUMAN_APPROVAL: "人工审批",
  INFERENCE_RUN: "推理运行",
  LOGICAL_MOVE: "逻辑移动",
  MODEL_INVOKE: "模型调用（旧版）",
  MODEL_WORKSTATION: "模型工作站（旧版）",
  POLLER_RUN: "轮询运行",
  SCRIPT_RUN: "脚本运行",
} satisfies WorkstationTypeLabelCatalog["labels"];

export function localizeWorkstationKindValue(
  value: ApiWorkstationKind | string,
  locale: string,
): string {
  const labelsByLocale: Record<string, WorkstationKindLabelCatalog["labels"]> =
    {
      en: WORKSTATION_KIND_LABELS_EN,
      ja: WORKSTATION_KIND_LABELS_JA,
      ko: WORKSTATION_KIND_LABELS_KO,
      "zh-CN": WORKSTATION_KIND_LABELS_ZH_CN,
    };

  return localizeEnumLabel({
    category: "kind",
    labels: labelsByLocale[locale] ?? WORKSTATION_KIND_LABELS_EN,
    locale,
    value,
  });
}

export function localizeWorkstationTypeValue(
  value: ApiWorkstationType | string,
  locale: string,
): string {
  const labelsByLocale: Record<string, WorkstationTypeLabelCatalog["labels"]> =
    {
      en: WORKSTATION_TYPE_LABELS_EN,
      ja: WORKSTATION_TYPE_LABELS_JA,
      ko: WORKSTATION_TYPE_LABELS_KO,
      "zh-CN": WORKSTATION_TYPE_LABELS_ZH_CN,
    };

  return localizeEnumLabel({
    category: "type",
    labels: labelsByLocale[locale] ?? WORKSTATION_TYPE_LABELS_EN,
    locale,
    value,
  });
}
