import type { components } from "../../../../api/generated/openapi";
import { type EnumLabelCatalog, localizeEnumLabel } from "../../../../i18n";

export type ApiRunnerID = components["schemas"]["RunnerID"];
export type ApiRunnerSelectionSource =
  components["schemas"]["RunnerSelectionSource"];

/** OpenAPI RunnerID enum order from the generated contract. */
export const OPENAPI_RUNNER_IDS = [
  "codex",
  "claude",
  "antigravity",
] as const satisfies readonly ApiRunnerID[];

type RunnerSelectionSourceLabelCatalog =
  EnumLabelCatalog<ApiRunnerSelectionSource>;

const RUNNER_SELECTION_SOURCE_LABELS_EN = {
  default: "Default",
  factory: "Factory",
  legacy_provider: "Worker provider",
  workstation: "Workstation",
} satisfies RunnerSelectionSourceLabelCatalog["labels"];

const RUNNER_SELECTION_SOURCE_LABELS_JA = {
  default: "既定",
  factory: "ファクトリー",
  legacy_provider: "ワーカー provider",
  workstation: "ワークステーション",
} satisfies RunnerSelectionSourceLabelCatalog["labels"];

const RUNNER_SELECTION_SOURCE_LABELS_KO = {
  default: "기본값",
  factory: "팩토리",
  legacy_provider: "워커 provider",
  workstation: "워크스테이션",
} satisfies RunnerSelectionSourceLabelCatalog["labels"];

const RUNNER_SELECTION_SOURCE_LABELS_ZH_CN = {
  default: "默认值",
  factory: "工厂",
  legacy_provider: "工作者 provider",
  workstation: "工作站",
} satisfies RunnerSelectionSourceLabelCatalog["labels"];

export function localizeRunnerSelectionSourceValue(
  value: ApiRunnerSelectionSource | string,
  locale: string,
): string {
  const labelsByLocale: Record<
    string,
    RunnerSelectionSourceLabelCatalog["labels"]
  > = {
    en: RUNNER_SELECTION_SOURCE_LABELS_EN,
    ja: RUNNER_SELECTION_SOURCE_LABELS_JA,
    ko: RUNNER_SELECTION_SOURCE_LABELS_KO,
    "zh-CN": RUNNER_SELECTION_SOURCE_LABELS_ZH_CN,
  };

  return localizeEnumLabel({
    category: "type",
    labels: labelsByLocale[locale] ?? RUNNER_SELECTION_SOURCE_LABELS_EN,
    locale,
    value,
  });
}

export function isOpenApiRunnerID(
  value: string | null | undefined,
): value is ApiRunnerID {
  if (!value) {
    return false;
  }

  return (OPENAPI_RUNNER_IDS as readonly string[]).includes(
    value.trim().toLowerCase(),
  );
}

export function normalizeRunnerID(value: string | null | undefined): string {
  return (value ?? "").trim().toLowerCase();
}
