import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "./messages";

export const ENUM_LOCALIZATION_CATEGORIES = [
  "status",
  "outcome",
  "kind",
  "type",
  "relation",
  "failureFamily",
] as const;

export type EnumLocalizationCategory =
  (typeof ENUM_LOCALIZATION_CATEGORIES)[number];

export const ENUM_LOCALIZATION_EXCLUSIONS = [
  "provider brand names",
  "model identifiers",
  "user-authored names",
  "IDs",
  "raw payload values",
  "saved canonical enum values",
  "generated API and config contract values",
] as const;

export interface EnumLocalizationContract {
  categoriesInScope: readonly EnumLocalizationCategory[];
  exclusions: readonly string[];
  fallbackBehavior: "localized-unknown-with-raw-value";
}

export interface EnumLocalizationMessages {
  categories: Record<EnumLocalizationCategory, string>;
  unknownValue: (params: {
    category: EnumLocalizationCategory;
    rawValue: string;
  }) => string;
}

export interface EnumLabelCatalog<TValue extends string> {
  category: EnumLocalizationCategory;
  labels: Partial<Record<TValue, string>>;
}

export const enumLocalizationContract: EnumLocalizationContract = {
  categoriesInScope: ENUM_LOCALIZATION_CATEGORIES,
  exclusions: ENUM_LOCALIZATION_EXCLUSIONS,
  fallbackBehavior: "localized-unknown-with-raw-value",
};

const enumCategoryLabelsEn: Record<EnumLocalizationCategory, string> = {
  status: "status",
  outcome: "outcome",
  kind: "kind",
  type: "type",
  relation: "relation",
  failureFamily: "failure family",
};

const enumCategoryLabelsZhCN: Record<EnumLocalizationCategory, string> = {
  status: "状态",
  outcome: "结果",
  kind: "种类",
  type: "类型",
  relation: "关系",
  failureFamily: "失败类别",
};

const enumLocalizationMessagesByLocale = {
  en: {
    categories: enumCategoryLabelsEn,
    unknownValue: ({ category, rawValue }) =>
      `Unknown ${enumCategoryLabelsEn[category]}: ${rawValue}`,
  },
  "zh-CN": {
    categories: enumCategoryLabelsZhCN,
    unknownValue: ({ category, rawValue }) =>
      `未知${enumCategoryLabelsZhCN[category]}：${rawValue}`,
  },
} satisfies LocalizedMessageCatalog<EnumLocalizationMessages>;

export function getEnumLocalizationMessages(
  locale?: string | null,
): EnumLocalizationMessages {
  return resolveLocalizedMessages(enumLocalizationMessagesByLocale, locale);
}

export function getLocalizedEnumCategoryLabel(
  category: EnumLocalizationCategory,
  locale?: string | null,
): string {
  return getEnumLocalizationMessages(locale).categories[category];
}

export function localizeEnumLabel<TValue extends string>(
  input: EnumLabelCatalog<TValue> & {
    locale?: string | null;
    value: TValue | string;
  },
): string {
  const value = input.value.trim();
  if (value.length === 0) {
    return getEnumLocalizationMessages(input.locale).unknownValue({
      category: input.category,
      rawValue: "empty",
    });
  }

  const knownLabel = input.labels[value as TValue];
  if (knownLabel) {
    return knownLabel;
  }

  return getEnumLocalizationMessages(input.locale).unknownValue({
    category: input.category,
    rawValue: value,
  });
}

export { enumLocalizationMessagesByLocale };
