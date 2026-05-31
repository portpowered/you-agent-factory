import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { WorkStateDetailMessages } from "./work-state-detail-types";

const workStateDetailMessagesByLocale = {
  en: {
    topologyDeleteAction: (workTypeName, stateName) =>
      `Delete ${workTypeName} ${stateName} work state`,
    topologyDeleteBlockedPrefix: "Work state cannot be deleted:",
    topologyDeleteHeading: "Factory graph",
    configurationEmpty:
      "This running factory definition does not include the selected work state.",
    configurationErrorPrefix: "Work state definition unavailable.",
    configurationLoading:
      "Loading the current factory definition for this work state.",
    editableConfigurationContractInvalidPrefix:
      "Work state configuration is invalid.",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable work state values.",
    editableConfigurationErrorPrefix: "Work state configuration unavailable.",
    editableConfigurationNameDuplicate: (stateName) =>
      `A work state named "${stateName}" already exists for this work type.`,
    editableConfigurationNameRequired:
      "Enter a work state name before saving this work state.",
    editableConfigurationSaveAction: "Save work state",
    editableConfigurationSaveBusyAction: "Saving work state...",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
  },
  ja: {
    topologyDeleteAction: (workTypeName, stateName) =>
      `ワーク状態 ${workTypeName} ${stateName} を削除`,
    topologyDeleteBlockedPrefix: "ワーク状態を削除できません:",
    topologyDeleteHeading: "ファクトリグラフ",
    configurationEmpty:
      "実行中のファクトリー定義に、選択したワーク状態が含まれていません。",
    configurationErrorPrefix: "ワーク状態定義を利用できません。",
    configurationLoading:
      "このワーク状態の現在のファクトリー定義を読み込んでいます。",
    editableConfigurationContractInvalidPrefix:
      "ワーク状態の構成が無効です。",
    editableConfigurationEmpty:
      "実行中のファクトリー定義に、編集可能なワーク状態の値がありません。",
    editableConfigurationErrorPrefix: "ワーク状態の構成を利用できません。",
    editableConfigurationNameDuplicate: (stateName) =>
      `このワークタイプには、すでに「${stateName}」というワーク状態があります。`,
    editableConfigurationNameRequired:
      "保存する前にワーク状態名を入力してください。",
    editableConfigurationSaveAction: "ワーク状態を保存",
    editableConfigurationSaveBusyAction: "ワーク状態を保存しています...",
    editableConfigurationSaveFallbackError:
      "実行中のファクトリーを保存できませんでした。",
  },
  ko: {
    topologyDeleteAction: (workTypeName, stateName) =>
      `작업 상태 ${workTypeName} ${stateName} 삭제`,
    topologyDeleteBlockedPrefix: "작업 상태를 삭제할 수 없습니다:",
    topologyDeleteHeading: "팩토리 그래프",
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 작업 상태가 포함되어 있지 않습니다.",
    configurationErrorPrefix: "작업 상태 정의를 사용할 수 없습니다.",
    configurationLoading:
      "이 작업 상태의 현재 팩토리 정의를 불러오는 중입니다.",
    editableConfigurationContractInvalidPrefix:
      "작업 상태 구성이 유효하지 않습니다.",
    editableConfigurationEmpty:
      "실행 중인 팩토리 정의에 편집 가능한 작업 상태 값이 없습니다.",
    editableConfigurationErrorPrefix: "작업 상태 구성을 사용할 수 없습니다.",
    editableConfigurationNameDuplicate: (stateName) =>
      `이 작업 유형에 이미 "${stateName}" 작업 상태가 있습니다.`,
    editableConfigurationNameRequired:
      "이 작업 상태를 저장하기 전에 작업 상태 이름을 입력하세요.",
    editableConfigurationSaveAction: "작업 상태 저장",
    editableConfigurationSaveBusyAction: "작업 상태 저장 중...",
    editableConfigurationSaveFallbackError:
      "실행 중인 팩토리를 저장할 수 없습니다.",
  },
  "zh-CN": {
    topologyDeleteAction: (workTypeName, stateName) =>
      `删除工作状态 ${workTypeName} ${stateName}`,
    topologyDeleteBlockedPrefix: "无法删除工作状态:",
    topologyDeleteHeading: "工厂图",
    configurationEmpty: "运行中的工厂定义不包含所选工作状态。",
    configurationErrorPrefix: "工作状态定义不可用。",
    configurationLoading: "正在加载此工作状态的当前工厂定义。",
    editableConfigurationContractInvalidPrefix: "工作状态配置无效。",
    editableConfigurationEmpty:
      "运行中的工厂定义未提供可编辑的工作状态值。",
    editableConfigurationErrorPrefix: "工作状态配置不可用。",
    editableConfigurationNameDuplicate: (stateName) =>
      `此工作类型已存在名为“${stateName}”的工作状态。`,
    editableConfigurationNameRequired: "保存此工作状态前请输入工作状态名称。",
    editableConfigurationSaveAction: "保存工作状态",
    editableConfigurationSaveBusyAction: "正在保存工作状态...",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
  },
} satisfies LocalizedMessages<WorkStateDetailMessages>;

export function getWorkStateDetailMessages(
  locale?: string | null,
): WorkStateDetailMessages {
  return resolveLocalizedMessages(workStateDetailMessagesByLocale, locale);
}
