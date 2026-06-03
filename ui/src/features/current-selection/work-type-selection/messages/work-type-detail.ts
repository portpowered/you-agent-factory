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
    collapseAction: "Collapse",
    editableConfigurationCollapseActionLabel:
      "Collapse work type configuration editor",
    editableConfigurationExpandActionLabel:
      "Expand work type configuration editor",
    editableConfigurationHeading: "Work type configuration",
    expandAction: "Expand",
    editableConfigurationContractInvalidPrefix:
      "Work type configuration is invalid.",
    editableConfigurationHandlingBehaviorMultipleDefault:
      "Only one work type can be the factory default.",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `A work type named "${workTypeName}" already exists in the running factory definition.`,
    editableConfigurationNameRequired:
      "Enter a work type name before saving this work type.",
    editableConfigurationSaveAction: "Save changes",
    editableConfigurationSaveBusyAction: "Saving...",
    editableConfigurationSaveConfirmationCancelAction: "Cancel",
    editableConfigurationSaveConfirmationConfirmAction: "Overwrite factory",
    editableConfigurationSaveConfirmationDescription:
      "Saving will overwrite the running factory definition with the work type name and CLI handling behavior in this draft.",
    editableConfigurationSaveConfirmationTitle:
      "Overwrite the running factory definition?",
    editableConfigurationSaveErrorPrefix: "Saving failed.",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
    editableConfigurationSaveStaleVersionDetail:
      "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
    editableConfigurationSaveSuccess: (workTypeName) =>
      `Running factory saved. "${workTypeName}" was refreshed to the saved definition.`,
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this work type.",
    handlingBehaviorDefaultHelper:
      "When marked default, this work type receives prompt text from simplified you run --factory flows.",
    handlingBehaviorDefaultLabel: "Mark as default work type",
    handlingBehaviorDefaultStatusLabel: "Default work type",
    localizeWorkStateType: localizeEnglishWorkStateType,
    selectWorkStateGraphNodeLabel: (stateName) =>
      `Select ${stateName} state on factory graph`,
    stateNameColumnLabel: "State",
    statesEmpty: "This work type does not define any states yet.",
    statesHeading: "States",
    stateTypeColumnLabel: "Type",
    workTypeNameLabel: "Work type",
  },
  ja: {
    configurationEmpty:
      "実行中のファクトリ定義に選択したワークタイプが含まれていません。",
    configurationErrorPrefix: "ワークタイプ定義を利用できません。",
    configurationLoading:
      "このワークタイプの現在のファクトリ定義を読み込んでいます。",
    collapseAction: "折りたたむ",
    editableConfigurationCollapseActionLabel:
      "ワークタイプ構成エディターを折りたたむ",
    editableConfigurationExpandActionLabel: "ワークタイプ構成エディターを展開",
    editableConfigurationHeading: "ワークタイプ構成",
    expandAction: "展開",
    editableConfigurationContractInvalidPrefix: "ワークタイプ設定が無効です。",
    editableConfigurationHandlingBehaviorMultipleDefault:
      "既定のファクトリワークタイプは 1 つだけにできます。",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `実行中のファクトリ定義には、すでに「${workTypeName}」というワークタイプが存在します。`,
    editableConfigurationNameRequired:
      "このワークタイプを保存する前にワークタイプ名を入力してください。",
    editableConfigurationSaveAction: "変更を保存",
    editableConfigurationSaveBusyAction: "保存中...",
    editableConfigurationSaveConfirmationCancelAction: "キャンセル",
    editableConfigurationSaveConfirmationConfirmAction: "ファクトリを上書き",
    editableConfigurationSaveConfirmationDescription:
      "保存すると、このドラフトのワークタイプ名と CLI 処理動作で実行中のファクトリ定義が上書きされます。",
    editableConfigurationSaveConfirmationTitle:
      "実行中のファクトリ定義を上書きしますか？",
    editableConfigurationSaveErrorPrefix: "保存に失敗しました。",
    editableConfigurationSaveFallbackError:
      "実行中のファクトリを保存できませんでした。",
    editableConfigurationSaveStaleVersionDetail:
      "最新の実行中ファクトリ値を再読み込みするか、エディターが更新された後にこのドラフトで再試行してください。",
    editableConfigurationSaveSuccess: (workTypeName) =>
      `実行中のファクトリを保存しました。「${workTypeName}」は保存済み定義に更新されました。`,
    editableConfigurationValidationStatus:
      "このワークタイプを保存する前に、強調表示されたフィールドを修正してください。",
    handlingBehaviorDefaultHelper:
      "既定にすると、このワークタイプは簡略化された you run --factory フローからのプロンプトを受け取ります。",
    handlingBehaviorDefaultLabel: "既定のワークタイプにする",
    handlingBehaviorDefaultStatusLabel: "既定のワークタイプ",
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
    selectWorkStateGraphNodeLabel: (stateName) =>
      `ファクトリグラフで ${stateName} 状態を選択`,
    stateNameColumnLabel: "状態",
    statesEmpty: "このワークタイプにはまだ状態が定義されていません。",
    statesHeading: "状態",
    stateTypeColumnLabel: "種類",
    workTypeNameLabel: "ワークタイプ",
  },
  ko: {
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 작업 유형이 포함되어 있지 않습니다.",
    configurationErrorPrefix: "작업 유형 정의를 사용할 수 없습니다.",
    configurationLoading:
      "이 작업 유형의 현재 팩토리 정의를 불러오는 중입니다.",
    collapseAction: "접기",
    editableConfigurationCollapseActionLabel: "작업 유형 구성 편집기 접기",
    editableConfigurationExpandActionLabel: "작업 유형 구성 편집기 펼치기",
    editableConfigurationHeading: "작업 유형 구성",
    expandAction: "펼치기",
    editableConfigurationContractInvalidPrefix:
      "작업 유형 구성이 유효하지 않습니다.",
    editableConfigurationHandlingBehaviorMultipleDefault:
      "팩토리 기본 작업 유형은 하나만 지정할 수 있습니다.",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `실행 중인 팩토리 정의에 "${workTypeName}" 작업 유형이 이미 있습니다.`,
    editableConfigurationNameRequired:
      "이 작업 유형을 저장하기 전에 작업 유형 이름을 입력하세요.",
    editableConfigurationSaveAction: "변경 사항 저장",
    editableConfigurationSaveBusyAction: "저장 중...",
    editableConfigurationSaveConfirmationCancelAction: "취소",
    editableConfigurationSaveConfirmationConfirmAction: "팩토리 덮어쓰기",
    editableConfigurationSaveConfirmationDescription:
      "저장하면 이 초안의 작업 유형 이름과 CLI 처리 동작으로 실행 중인 팩토리 정의가 덮어씌워집니다.",
    editableConfigurationSaveConfirmationTitle:
      "실행 중인 팩토리 정의를 덮어쓸까요?",
    editableConfigurationSaveErrorPrefix: "저장하지 못했습니다.",
    editableConfigurationSaveFallbackError:
      "실행 중인 팩토리를 저장할 수 없습니다.",
    editableConfigurationSaveStaleVersionDetail:
      "최신 실행 중 팩토리 값을 다시 불러오거나 편집기가 새로고침된 뒤 이 초안으로 다시 시도하세요.",
    editableConfigurationSaveSuccess: (workTypeName) =>
      `실행 중인 팩토리를 저장했습니다. "${workTypeName}"이(가) 저장된 정의로 새로고침되었습니다.`,
    editableConfigurationValidationStatus:
      "이 작업 유형을 저장하기 전에 강조 표시된 필드를 수정하세요.",
    handlingBehaviorDefaultHelper:
      "기본으로 지정하면 이 작업 유형은 간소화된 you run --factory 흐름의 프롬프트를 받습니다.",
    handlingBehaviorDefaultLabel: "기본 작업 유형으로 지정",
    handlingBehaviorDefaultStatusLabel: "기본 작업 유형",
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
    selectWorkStateGraphNodeLabel: (stateName) =>
      `팩토리 그래프에서 ${stateName} 상태 선택`,
    stateNameColumnLabel: "상태",
    statesEmpty: "이 작업 유형에는 아직 정의된 상태가 없습니다.",
    statesHeading: "상태",
    stateTypeColumnLabel: "유형",
    workTypeNameLabel: "작업 유형",
  },
  "zh-CN": {
    configurationEmpty: "运行中的工厂定义不包含所选工作类型。",
    configurationErrorPrefix: "工作类型定义不可用。",
    configurationLoading: "正在加载此工作类型的当前工厂定义。",
    collapseAction: "收起",
    editableConfigurationCollapseActionLabel: "收起工作类型配置编辑器",
    editableConfigurationExpandActionLabel: "展开工作类型配置编辑器",
    editableConfigurationHeading: "工作类型配置",
    expandAction: "展开",
    editableConfigurationContractInvalidPrefix: "工作类型配置无效。",
    editableConfigurationHandlingBehaviorMultipleDefault:
      "只能将一个工作类型设为工厂默认类型。",
    editableConfigurationNameDuplicate: (workTypeName: string) =>
      `运行中的工厂定义中已存在名为“${workTypeName}”的工作类型。`,
    editableConfigurationNameRequired: "保存此工作类型前请输入工作类型名称。",
    editableConfigurationSaveAction: "保存更改",
    editableConfigurationSaveBusyAction: "正在保存...",
    editableConfigurationSaveConfirmationCancelAction: "取消",
    editableConfigurationSaveConfirmationConfirmAction: "覆盖工厂",
    editableConfigurationSaveConfirmationDescription:
      "保存将使用此草稿中的工作类型名称和 CLI 处理行为覆盖运行中的工厂定义。",
    editableConfigurationSaveConfirmationTitle: "覆盖运行中的工厂定义？",
    editableConfigurationSaveErrorPrefix: "保存失败。",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
    editableConfigurationSaveStaleVersionDetail:
      "请重新加载最新的运行中工厂值，或在编辑器刷新后使用此草稿重试。",
    editableConfigurationSaveSuccess: (workTypeName) =>
      `已保存运行中的工厂。“${workTypeName}”已刷新为保存后的定义。`,
    editableConfigurationValidationStatus: "保存此工作类型前请修正高亮字段。",
    handlingBehaviorDefaultHelper:
      "标记为默认后，此工作类型将接收来自简化 you run --factory 流程的提示文本。",
    handlingBehaviorDefaultLabel: "标记为默认工作类型",
    handlingBehaviorDefaultStatusLabel: "默认工作类型",
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
    selectWorkStateGraphNodeLabel: (stateName) =>
      `在工厂 graph 中选择 ${stateName} 状态`,
    stateNameColumnLabel: "状态",
    statesEmpty: "此工作类型尚未定义任何状态。",
    statesHeading: "状态",
    stateTypeColumnLabel: "类型",
    workTypeNameLabel: "工作类型",
  },
} satisfies LocalizedMessages<WorkTypeDetailMessages>;

export function getWorkTypeDetailMessages(
  locale?: string | null,
): WorkTypeDetailMessages {
  return resolveLocalizedMessages(workTypeDetailMessagesByLocale, locale);
}
