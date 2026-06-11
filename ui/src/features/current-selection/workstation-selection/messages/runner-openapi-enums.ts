import type { components } from "../../../../api/generated/openapi";
import { type EnumLabelCatalog, localizeEnumLabel } from "../../../../i18n";

export type ApiModelProviderSelection =
  components["schemas"]["ModelProviderSelection"];
export type ApiModelProviderSelectionSource =
  components["schemas"]["ModelProviderSelectionSource"];

/** Backward-compatible alias for workstation editing surfaces. */
export type ApiRunnerID = ApiModelProviderSelection;

/** OpenAPI ModelProviderSelection enum order from the generated contract. */
export const OPENAPI_MODEL_PROVIDER_SELECTIONS = [
  "DEFAULT",
  "CLAUDE",
  "CODEX",
  "CURSOR",
  "GEMINI",
  "KIRO",
  "OPENCODE",
] as const satisfies readonly ApiModelProviderSelection[];

/** Backward-compatible alias for existing runner-named imports. */
export const OPENAPI_RUNNER_IDS = OPENAPI_MODEL_PROVIDER_SELECTIONS;

type ModelProviderSelectionSourceLabelCatalog =
  EnumLabelCatalog<ApiModelProviderSelectionSource>;

const MODEL_PROVIDER_SELECTION_SOURCE_LABELS_EN = {
  operator_default: "Operator default",
  factory: "Factory",
  worker: "Worker provider",
  workstation: "Workstation",
} satisfies ModelProviderSelectionSourceLabelCatalog["labels"];

const MODEL_PROVIDER_SELECTION_SOURCE_LABELS_JA = {
  operator_default: "オペレーター既定",
  factory: "ファクトリー",
  worker: "ワーカー provider",
  workstation: "ワークステーション",
} satisfies ModelProviderSelectionSourceLabelCatalog["labels"];

const MODEL_PROVIDER_SELECTION_SOURCE_LABELS_KO = {
  operator_default: "운영자 기본값",
  factory: "팩토리",
  worker: "워커 provider",
  workstation: "워크스테이션",
} satisfies ModelProviderSelectionSourceLabelCatalog["labels"];

const MODEL_PROVIDER_SELECTION_SOURCE_LABELS_ZH_CN = {
  operator_default: "操作者默认值",
  factory: "工厂",
  worker: "工作者 provider",
  workstation: "工作站",
} satisfies ModelProviderSelectionSourceLabelCatalog["labels"];

export function localizeModelProviderSelectionSourceValue(
  value: ApiModelProviderSelectionSource | string,
  locale: string,
): string {
  const labelsByLocale: Record<
    string,
    ModelProviderSelectionSourceLabelCatalog["labels"]
  > = {
    en: MODEL_PROVIDER_SELECTION_SOURCE_LABELS_EN,
    ja: MODEL_PROVIDER_SELECTION_SOURCE_LABELS_JA,
    ko: MODEL_PROVIDER_SELECTION_SOURCE_LABELS_KO,
    "zh-CN": MODEL_PROVIDER_SELECTION_SOURCE_LABELS_ZH_CN,
  };

  return localizeEnumLabel({
    category: "type",
    labels: labelsByLocale[locale] ?? MODEL_PROVIDER_SELECTION_SOURCE_LABELS_EN,
    locale,
    value,
  });
}

/** Backward-compatible alias for existing runner-named imports. */
export const localizeRunnerSelectionSourceValue =
  localizeModelProviderSelectionSourceValue;

export function isOpenApiModelProviderSelection(
  value: string | null | undefined,
): value is ApiModelProviderSelection {
  if (!value) {
    return false;
  }

  return (OPENAPI_MODEL_PROVIDER_SELECTIONS as readonly string[]).includes(
    value.trim().toUpperCase(),
  );
}

/** Backward-compatible alias for existing runner-named imports. */
export const isOpenApiRunnerID = isOpenApiModelProviderSelection;

export function normalizeModelProviderSelection(
  value: string | null | undefined,
): string {
  return (value ?? "").trim().toUpperCase();
}

/** Backward-compatible alias for existing runner-named imports. */
export const normalizeRunnerID = normalizeModelProviderSelection;
