import type { components } from "../../../../api/generated/openapi";
import {
  localizeEnumLabel,
  type EnumLabelCatalog,
} from "../../../../i18n";

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
  CLASSIFIER_WORKSTATION: "Classifier workstation",
  LOGICAL_MOVE: "Logical move",
  MODEL_INVOKE: "Model invoke",
  MODEL_WORKSTATION: "Model workstation",
} satisfies WorkstationTypeLabelCatalog["labels"];

const WORKSTATION_TYPE_LABELS_JA = {
  CLASSIFIER_WORKSTATION: "分類ワークステーション",
  LOGICAL_MOVE: "論理移動",
  MODEL_INVOKE: "モデル呼び出し",
  MODEL_WORKSTATION: "モデルワークステーション",
} satisfies WorkstationTypeLabelCatalog["labels"];

const WORKSTATION_TYPE_LABELS_KO = {
  CLASSIFIER_WORKSTATION: "분류 워크스테이션",
  LOGICAL_MOVE: "논리 이동",
  MODEL_INVOKE: "모델 호출",
  MODEL_WORKSTATION: "모델 워크스테이션",
} satisfies WorkstationTypeLabelCatalog["labels"];

const WORKSTATION_TYPE_LABELS_ZH_CN = {
  CLASSIFIER_WORKSTATION: "分类工作站",
  LOGICAL_MOVE: "逻辑移动",
  MODEL_INVOKE: "模型调用",
  MODEL_WORKSTATION: "模型工作站",
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
