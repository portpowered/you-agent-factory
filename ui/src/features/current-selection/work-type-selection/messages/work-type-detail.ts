import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { WorkTypeDetailMessages } from "./work-type-detail-types";

const workTypeDetailMessagesByLocale = {
  en: {
    configurationEmpty:
      "This running factory definition does not include the selected work type.",
    editableConfigurationContractInvalidPrefix:
      "Work type configuration is invalid.",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `A work type named "${workTypeName}" already exists in the running factory definition.`,
    editableConfigurationNameRequired:
      "Enter a work type name before saving this work type.",
    topologyDeleteAction: (workTypeName) => `Delete ${workTypeName} work type`,
    topologyDeleteBlockedPrefix: "Work type cannot be deleted:",
    topologyDeleteHeading: "Factory graph",
  },
  ja: {
    configurationEmpty:
      "実行中のファクトリ定義に選択したワークタイプが含まれていません。",
    editableConfigurationContractInvalidPrefix: "ワークタイプ設定が無効です。",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `実行中のファクトリ定義には、すでに「${workTypeName}」というワークタイプが存在します。`,
    editableConfigurationNameRequired:
      "このワークタイプを保存する前にワークタイプ名を入力してください。",
    topologyDeleteAction: (workTypeName) => `ワークタイプ ${workTypeName} を削除`,
    topologyDeleteBlockedPrefix: "ワークタイプを削除できません:",
    topologyDeleteHeading: "ファクトリグラフ",
  },
  ko: {
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 작업 유형이 포함되어 있지 않습니다.",
    editableConfigurationContractInvalidPrefix:
      "작업 유형 구성이 유효하지 않습니다.",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `실행 중인 팩토리 정의에 "${workTypeName}" 작업 유형이 이미 있습니다.`,
    editableConfigurationNameRequired:
      "이 작업 유형을 저장하기 전에 작업 유형 이름을 입력하세요.",
    topologyDeleteAction: (workTypeName) => `작업 유형 ${workTypeName} 삭제`,
    topologyDeleteBlockedPrefix: "작업 유형을 삭제할 수 없습니다:",
    topologyDeleteHeading: "팩토리 그래프",
  },
  "zh-CN": {
    configurationEmpty: "运行中的工厂定义不包含所选工作类型。",
    editableConfigurationContractInvalidPrefix: "工作类型配置无效。",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `运行中的工厂定义中已存在名为“${workTypeName}”的工作类型。`,
    editableConfigurationNameRequired: "保存此工作类型前请输入工作类型名称。",
    topologyDeleteAction: (workTypeName) => `删除工作类型 ${workTypeName}`,
    topologyDeleteBlockedPrefix: "无法删除工作类型:",
    topologyDeleteHeading: "工厂图",
  },
} satisfies LocalizedMessages<WorkTypeDetailMessages>;

export function getWorkTypeDetailMessages(
  locale?: string | null,
): WorkTypeDetailMessages {
  return resolveLocalizedMessages(workTypeDetailMessagesByLocale, locale);
}
