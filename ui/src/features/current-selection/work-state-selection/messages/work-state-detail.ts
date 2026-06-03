import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import { localizeWorkStateType } from "./work-state-detail-enums";
import type { WorkStateDetailMessages } from "./work-state-detail-types";

type WorkStateDetailCatalogMessages = Omit<
  WorkStateDetailMessages,
  "localizeWorkStateType"
>;

const workStateDetailMessagesByLocale = {
  en: {
    configurationEmpty:
      "This running factory definition does not include the selected work state.",
    configurationErrorPrefix: "Work state definition unavailable.",
    configurationLoading:
      "Loading the current factory definition for this work state.",
    collapseAction: "Collapse",
    discardDraftAction: "Discard local changes",
    editableConfigurationCollapseActionLabel:
      "Collapse work state configuration editor",
    editableConfigurationExpandActionLabel:
      "Expand work state configuration editor",
    expandAction: "Expand",
    editableConfigurationContractInvalidPrefix:
      "Work state configuration is invalid.",
    editableConfigurationDirtyStatus:
      "You have unsaved changes for this work state.",
    editableConfigurationDraftNote:
      "Changes stay local to this edit session until you save the running factory.",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable work state values.",
    editableConfigurationErrorPrefix: "Work state configuration unavailable.",
    editableConfigurationHeading: "Work state configuration",
    editableConfigurationLoading: "Loading editable work state configuration.",
    editableConfigurationNameDuplicate: (stateName) =>
      `A work state named "${stateName}" already exists for this work type.`,
    editableConfigurationNameRequired:
      "Enter a work state name before saving this work state.",
    editableConfigurationSaveAction: "Save work state",
    editableConfigurationSaveBusyAction: "Saving work state...",
    editableConfigurationSaveDisabledValidationDetail:
      "Save stays disabled until the highlighted work state fields are valid.",
    editableConfigurationSaveErrorPrefix: "Saving failed.",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
    editableConfigurationSaveStaleVersionDetail:
      "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
    editableConfigurationSaveSuccess: (stateName) =>
      `Running factory saved. ${stateName} was updated in the running factory definition.`,
    editableConfigurationValidationStatus:
      "Fix the highlighted work state fields before saving.",
    nameFieldLabel: "State name",
    typeFieldLabel: "Lifecycle type",
  },
  ja: {
    configurationEmpty:
      "実行中のファクトリー定義に、選択したワーク状態が含まれていません。",
    configurationErrorPrefix: "ワーク状態定義を利用できません。",
    configurationLoading:
      "このワーク状態の現在のファクトリー定義を読み込んでいます。",
    collapseAction: "折りたたむ",
    discardDraftAction: "ローカルの変更を破棄",
    editableConfigurationCollapseActionLabel:
      "ワーク状態構成エディターを折りたたむ",
    editableConfigurationExpandActionLabel: "ワーク状態構成エディターを展開",
    expandAction: "展開",
    editableConfigurationContractInvalidPrefix: "ワーク状態の構成が無効です。",
    editableConfigurationDirtyStatus:
      "このワーク状態に未保存の変更があります。",
    editableConfigurationDraftNote:
      "実行中のファクトリーを保存するまで、変更はこの編集セッション内に留まります。",
    editableConfigurationEmpty:
      "実行中のファクトリー定義に、編集可能なワーク状態の値がありません。",
    editableConfigurationErrorPrefix: "ワーク状態の構成を利用できません。",
    editableConfigurationHeading: "ワーク状態の構成",
    editableConfigurationLoading:
      "編集可能なワーク状態の構成を読み込んでいます。",
    editableConfigurationNameDuplicate: (stateName) =>
      `このワークタイプには、すでに「${stateName}」というワーク状態があります。`,
    editableConfigurationNameRequired:
      "保存する前にワーク状態名を入力してください。",
    editableConfigurationSaveAction: "ワーク状態を保存",
    editableConfigurationSaveBusyAction: "ワーク状態を保存しています...",
    editableConfigurationSaveDisabledValidationDetail:
      "強調表示されたワーク状態フィールドが有効になるまで保存は無効のままです。",
    editableConfigurationSaveErrorPrefix: "保存に失敗しました。",
    editableConfigurationSaveFallbackError:
      "実行中のファクトリーを保存できませんでした。",
    editableConfigurationSaveStaleVersionDetail:
      "最新の実行中ファクトリー値を再読み込みするか、この下書きを保持してエディター更新後に再試行してください。",
    editableConfigurationSaveSuccess: (stateName) =>
      `実行中のファクトリーを保存しました。${stateName} を実行中のファクトリー定義で更新しました。`,
    editableConfigurationValidationStatus:
      "保存する前に強調表示されたワーク状態フィールドを修正してください。",
    nameFieldLabel: "状態名",
    typeFieldLabel: "ライフサイクル種別",
  },
  ko: {
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 작업 상태가 포함되어 있지 않습니다.",
    configurationErrorPrefix: "작업 상태 정의를 사용할 수 없습니다.",
    configurationLoading:
      "이 작업 상태의 현재 팩토리 정의를 불러오는 중입니다.",
    collapseAction: "접기",
    discardDraftAction: "로컬 변경 사항 취소",
    editableConfigurationCollapseActionLabel: "작업 상태 구성 편집기 접기",
    editableConfigurationExpandActionLabel: "작업 상태 구성 편집기 펼치기",
    expandAction: "펼치기",
    editableConfigurationContractInvalidPrefix:
      "작업 상태 구성이 유효하지 않습니다.",
    editableConfigurationDirtyStatus:
      "이 작업 상태에 저장되지 않은 변경 사항이 있습니다.",
    editableConfigurationDraftNote:
      "실행 중인 팩토리를 저장할 때까지 변경 사항은 이 편집 세션에만 유지됩니다.",
    editableConfigurationEmpty:
      "실행 중인 팩토리 정의에 편집 가능한 작업 상태 값이 없습니다.",
    editableConfigurationErrorPrefix: "작업 상태 구성을 사용할 수 없습니다.",
    editableConfigurationHeading: "작업 상태 구성",
    editableConfigurationLoading:
      "편집 가능한 작업 상태 구성을 불러오는 중입니다.",
    editableConfigurationNameDuplicate: (stateName) =>
      `이 작업 유형에 이미 "${stateName}" 작업 상태가 있습니다.`,
    editableConfigurationNameRequired:
      "이 작업 상태를 저장하기 전에 작업 상태 이름을 입력하세요.",
    editableConfigurationSaveAction: "작업 상태 저장",
    editableConfigurationSaveBusyAction: "작업 상태 저장 중...",
    editableConfigurationSaveDisabledValidationDetail:
      "강조 표시된 작업 상태 필드가 유효해질 때까지 저장이 비활성화됩니다.",
    editableConfigurationSaveErrorPrefix: "저장에 실패했습니다.",
    editableConfigurationSaveFallbackError:
      "실행 중인 팩토리를 저장할 수 없습니다.",
    editableConfigurationSaveStaleVersionDetail:
      "최신 실행 중 팩토리 값을 다시 불러오거나 이 초안을 유지한 뒤 편집기가 새로고침된 후 다시 시도하세요.",
    editableConfigurationSaveSuccess: (stateName) =>
      `실행 중인 팩토리를 저장했습니다. ${stateName}이(가) 실행 중 팩토리 정의에서 업데이트되었습니다.`,
    editableConfigurationValidationStatus:
      "저장하기 전에 강조 표시된 작업 상태 필드를 수정하세요.",
    nameFieldLabel: "상태 이름",
    typeFieldLabel: "수명 주기 유형",
  },
  "zh-CN": {
    configurationEmpty: "运行中的工厂定义不包含所选工作状态。",
    configurationErrorPrefix: "工作状态定义不可用。",
    configurationLoading: "正在加载此工作状态的当前工厂定义。",
    collapseAction: "收起",
    discardDraftAction: "放弃本地更改",
    editableConfigurationCollapseActionLabel: "收起工作状态配置编辑器",
    editableConfigurationExpandActionLabel: "展开工作状态配置编辑器",
    expandAction: "展开",
    editableConfigurationContractInvalidPrefix: "工作状态配置无效。",
    editableConfigurationDirtyStatus: "此工作状态有未保存的更改。",
    editableConfigurationDraftNote:
      "在保存运行中的工厂之前，更改仅保留在此编辑会话中。",
    editableConfigurationEmpty: "运行中的工厂定义未提供可编辑的工作状态值。",
    editableConfigurationErrorPrefix: "工作状态配置不可用。",
    editableConfigurationHeading: "工作状态配置",
    editableConfigurationLoading: "正在加载可编辑的工作状态配置。",
    editableConfigurationNameDuplicate: (stateName) =>
      `此工作类型已存在名为“${stateName}”的工作状态。`,
    editableConfigurationNameRequired: "保存此工作状态前请输入工作状态名称。",
    editableConfigurationSaveAction: "保存工作状态",
    editableConfigurationSaveBusyAction: "正在保存工作状态...",
    editableConfigurationSaveDisabledValidationDetail:
      "在突出显示的工作状态字段有效之前，保存将保持禁用。",
    editableConfigurationSaveErrorPrefix: "保存失败。",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
    editableConfigurationSaveStaleVersionDetail:
      "重新加载最新的运行中工厂值，或保留此草稿并在编辑器刷新后重试。",
    editableConfigurationSaveSuccess: (stateName) =>
      `已保存运行中的工厂。${stateName} 已在运行中的工厂定义中更新。`,
    editableConfigurationValidationStatus:
      "保存前请修复突出显示的工作状态字段。",
    nameFieldLabel: "状态名称",
    typeFieldLabel: "生命周期类型",
  },
} satisfies LocalizedMessages<WorkStateDetailCatalogMessages>;

export function getWorkStateDetailMessages(
  locale?: string | null,
): WorkStateDetailMessages {
  const messages = resolveLocalizedMessages(
    workStateDetailMessagesByLocale,
    locale,
  );

  return {
    ...messages,
    localizeWorkStateType: (stateType) =>
      localizeWorkStateType(stateType, locale),
  };
}
