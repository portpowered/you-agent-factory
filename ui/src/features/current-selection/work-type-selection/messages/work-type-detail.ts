import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { WorkTypeDetailMessages } from "./work-type-detail-types";

function localizeEnglishWorkStateType(
  workStateType: Parameters<WorkTypeDetailMessages["localizeWorkStateType"]>[0],
): string {
  switch (workStateType) {
    case "INITIAL":
      return "Initial";
    case "PROCESSING":
      return "Processing";
    case "TERMINAL":
      return "Completed";
    case "FAILED":
      return "Failed";
  }
}

const workTypeDetailMessagesByLocale = {
  en: {
    configurationEmpty:
      "This running factory definition does not include the selected work type.",
    configurationErrorPrefix: "Work type definition unavailable.",
    configurationLoading:
      "Loading the current factory definition for this work type.",
    editableConfigurationContractInvalidPrefix:
      "Work type configuration is invalid.",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `A work type named "${workTypeName}" already exists in the running factory definition.`,
    editableConfigurationNameRequired:
      "Enter a work type name before saving this work type.",
    localizeWorkStateType: localizeEnglishWorkStateType,
    stateNameColumnLabel: "State",
    statesEmpty: "This work type does not define any states yet.",
    statesHeading: "States",
    stateTypeColumnLabel: "Type",
    topologyDeleteAction: (workTypeName) => `Delete ${workTypeName} work type`,
    topologyDeleteBlockedPrefix: "Work type cannot be deleted:",
    topologyDeleteHeading: "Factory graph",
    workTypeNameLabel: "Work type",
  },
  ja: {
    configurationEmpty:
      "実行中のファクトリ定義に選択したワークタイプが含まれていません。",
    configurationErrorPrefix: "ワークタイプ定義を利用できません。",
    configurationLoading:
      "このワークタイプの現在のファクトリ定義を読み込んでいます。",
    editableConfigurationContractInvalidPrefix: "ワークタイプ設定が無効です。",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `実行中のファクトリ定義には、すでに「${workTypeName}」というワークタイプが存在します。`,
    editableConfigurationNameRequired:
      "このワークタイプを保存する前にワークタイプ名を入力してください。",
    localizeWorkStateType: (workStateType) => {
      switch (workStateType) {
        case "INITIAL":
          return "初期";
        case "PROCESSING":
          return "処理中";
        case "TERMINAL":
          return "完了";
        case "FAILED":
          return "失敗";
      }
    },
    stateNameColumnLabel: "状態",
    statesEmpty: "このワークタイプにはまだ状態が定義されていません。",
    statesHeading: "状態",
    stateTypeColumnLabel: "種類",
    topologyDeleteAction: (workTypeName) => `ワークタイプ ${workTypeName} を削除`,
    topologyDeleteBlockedPrefix: "ワークタイプを削除できません:",
    topologyDeleteHeading: "ファクトリグラフ",
    workTypeNameLabel: "ワークタイプ",
  },
  ko: {
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 작업 유형이 포함되어 있지 않습니다.",
    configurationErrorPrefix: "작업 유형 정의를 사용할 수 없습니다.",
    configurationLoading:
      "이 작업 유형의 현재 팩토리 정의를 불러오는 중입니다.",
    editableConfigurationContractInvalidPrefix:
      "작업 유형 구성이 유효하지 않습니다.",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `실행 중인 팩토리 정의에 "${workTypeName}" 작업 유형이 이미 있습니다.`,
    editableConfigurationNameRequired:
      "이 작업 유형을 저장하기 전에 작업 유형 이름을 입력하세요.",
    localizeWorkStateType: (workStateType) => {
      switch (workStateType) {
        case "INITIAL":
          return "초기";
        case "PROCESSING":
          return "처리 중";
        case "TERMINAL":
          return "완료";
        case "FAILED":
          return "실패";
      }
    },
    stateNameColumnLabel: "상태",
    statesEmpty: "이 작업 유형에는 아직 정의된 상태가 없습니다.",
    statesHeading: "상태",
    stateTypeColumnLabel: "유형",
    topologyDeleteAction: (workTypeName) => `작업 유형 ${workTypeName} 삭제`,
    topologyDeleteBlockedPrefix: "작업 유형을 삭제할 수 없습니다:",
    topologyDeleteHeading: "팩토리 그래프",
    workTypeNameLabel: "작업 유형",
  },
  "zh-CN": {
    configurationEmpty: "运行中的工厂定义不包含所选工作类型。",
    configurationErrorPrefix: "工作类型定义不可用。",
    configurationLoading: "正在加载此工作类型的当前工厂定义。",
    editableConfigurationContractInvalidPrefix: "工作类型配置无效。",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `运行中的工厂定义中已存在名为“${workTypeName}”的工作类型。`,
    editableConfigurationNameRequired: "保存此工作类型前请输入工作类型名称。",
    localizeWorkStateType: (workStateType) => {
      switch (workStateType) {
        case "INITIAL":
          return "初始";
        case "PROCESSING":
          return "处理中";
        case "TERMINAL":
          return "已完成";
        case "FAILED":
          return "失败";
      }
    },
    stateNameColumnLabel: "状态",
    statesEmpty: "此工作类型尚未定义任何状态。",
    statesHeading: "状态",
    stateTypeColumnLabel: "类型",
    topologyDeleteAction: (workTypeName) => `删除工作类型 ${workTypeName}`,
    topologyDeleteBlockedPrefix: "无法删除工作类型:",
    topologyDeleteHeading: "工厂图",
    workTypeNameLabel: "工作类型",
  },
} satisfies LocalizedMessages<WorkTypeDetailMessages>;

export function getWorkTypeDetailMessages(
  locale?: string | null,
): WorkTypeDetailMessages {
  return resolveLocalizedMessages(workTypeDetailMessagesByLocale, locale);
}
