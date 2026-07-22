import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { CliRelationship } from "../lib/cli-manifest-types";

export interface CliControlMessages {
  addValue: (label: string) => string;
  cardinalityError: (
    label: string,
    minimum: number,
    maximum: number | null,
  ) => string;
  defaultValue: (value: string) => string;
  inherited: string;
  optionalChoice: string;
  relationshipError: (kind: CliRelationship["kind"], labels: string) => string;
  relationshipGuidance: (labels: string) => string;
  removeValue: (label: string, position: number) => string;
  required: string;
  valuePosition: (label: string, position: number) => string;
}

const cliControlMessagesByLocale = {
  en: {
    addValue: (label) => `Add another ${label} value`,
    cardinalityError: (label, minimum, maximum) =>
      `${label} requires between ${minimum} and ${maximum ?? "any number of"} values.`,
    defaultValue: (value) => `Default: ${value}`,
    inherited: "Inherited global input",
    optionalChoice: "Not specified",
    relationshipError: (kind, labels) => {
      switch (kind) {
        case "at-least-one":
          return `Choose at least one of ${labels}.`;
        case "conditional":
          return `This value requires ${labels}.`;
        case "required-together":
          return `This value must be provided together with ${labels}.`;
        case "conflict":
        case "mutually-exclusive":
          return `This value conflicts with ${labels}.`;
      }
    },
    relationshipGuidance: (labels) => `Related inputs: ${labels}.`,
    removeValue: (label, position) => `Remove ${label} value ${position}`,
    required: "Required",
    valuePosition: (label, position) => `${label} value ${position}`,
  },
  "zh-CN": {
    addValue: (label) => `添加另一个 ${label} 值`,
    cardinalityError: (label, minimum, maximum) =>
      `${label} 需要 ${minimum} 到 ${maximum ?? "任意多个"} 个值。`,
    defaultValue: (value) => `默认值：${value}`,
    inherited: "继承的全局输入",
    optionalChoice: "未指定",
    relationshipError: (kind, labels) => {
      switch (kind) {
        case "at-least-one":
          return `请至少选择 ${labels} 中的一项。`;
        case "conditional":
          return `此值需要同时提供 ${labels}。`;
        case "required-together":
          return `此值必须与 ${labels} 一起提供。`;
        case "conflict":
        case "mutually-exclusive":
          return `此值与 ${labels} 冲突。`;
      }
    },
    relationshipGuidance: (labels) => `相关输入：${labels}。`,
    removeValue: (label, position) => `移除 ${label} 的第 ${position} 个值`,
    required: "必填",
    valuePosition: (label, position) => `${label} 的第 ${position} 个值`,
  },
} satisfies LocalizedMessageCatalog<CliControlMessages>;

export function getCliControlMessages(locale?: string | null) {
  return resolveLocalizedMessages(cliControlMessagesByLocale, locale);
}
