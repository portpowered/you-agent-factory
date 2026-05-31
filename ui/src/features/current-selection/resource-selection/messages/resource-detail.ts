import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import { getResourceDetailEnumMessages } from "./resource-detail-enums";
import type { ResourceDetailMessages } from "./resource-detail-types";

type ResourceDetailCatalogMessages = Omit<
  ResourceDetailMessages,
  "localizeResourceType"
>;

const resourceDetailMessagesByLocale = {
  en: {
    backendFieldLabel: "Backend",
    capacityFieldLabel: "Capacity",
    collapseAction: "Collapse",
    configurationEmpty:
      "This running factory definition does not include the selected resource.",
    configurationErrorPrefix: "Resource definition unavailable.",
    configurationLoading:
      "Loading the current factory definition for this resource.",
    editableConfigurationCapacityInvalid:
      "Resource capacity must be a whole number greater than zero.",
    editableConfigurationCollapseActionLabel:
      "Collapse resource configuration editor",
    editableConfigurationContractInvalidPrefix:
      "Resource configuration is invalid.",
    editableConfigurationDirtyStatus:
      "You have unsaved changes for this resource.",
    editableConfigurationDraftNote:
      "Changes stay local to this edit session until you save the running factory.",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable resource values.",
    editableConfigurationErrorPrefix: "Resource configuration unavailable.",
    editableConfigurationExpandActionLabel:
      "Expand resource configuration editor",
    editableConfigurationHeading: "Resource configuration",
    editableConfigurationLoading: "Loading editable resource configuration.",
    editableConfigurationNameDuplicate: (resourceName) =>
      `A resource named "${resourceName}" already exists in the running factory definition.`,
    editableConfigurationNameRequired:
      "Enter a resource name before saving this resource.",
    editableConfigurationOverwriteWarning: (fields) =>
      `The running factory changed after you started editing. Saving now will overwrite newer server values for ${fields}.`,
    editableConfigurationOverwriteWarningDetail:
      "Review the latest runtime values before saving, or keep editing if this draft should replace them.",
    editableConfigurationSaveAction: "Save resource",
    editableConfigurationSaveBusyAction: "Saving resource...",
    editableConfigurationSaveDisabledValidationDetail:
      "Save stays disabled until the highlighted resource fields are valid.",
    editableConfigurationSaveErrorPrefix: "Saving failed.",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
    editableConfigurationSaveStaleVersionDetail:
      "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
    editableConfigurationSaveSuccess: (resourceName) =>
      `Running factory saved. ${resourceName} was updated in the running factory definition.`,
    editableConfigurationServerFieldChangedHint:
      "The running factory changed this field while you were editing. Discard local changes to use the latest server-backed value.",
    editableConfigurationSharedImpactWarning: (
      resourceName,
      workerNames,
      workstationNames,
    ) =>
      `Saving ${resourceName} updates every worker and workstation that references this resource, including workers ${workerNames} and workstations ${workstationNames}.`,
    editableConfigurationSharedImpactWarningDetail:
      "Capacity and resource metadata apply anywhere this resource name is referenced.",
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this resource.",
    expandAction: "Expand",
    loadPolicyFieldLabel: "Load policy",
    modelFieldLabel: "Model",
    nameFieldLabel: "Name",
    notConfiguredValue: "Not configured",
    providerFieldLabel: "Provider",
    referencingWorkersEmpty:
      "No workers in the running factory definition require this resource.",
    referencingWorkersHeading: "Referencing workers",
    referencingWorkstationsEmpty:
      "No workstations in the running factory definition consume this resource.",
    referencingWorkstationsHeading: "Referencing workstations",
    resetToLatestAction: "Discard local changes",
    summaryHeading: "Summary",
    tokenCountFieldLabel: "Available tokens",
    typeFieldLabel: "Type",
    unknownTypeValue: "Unknown type",
  },
  ja: {
    backendFieldLabel: "バックエンド",
    capacityFieldLabel: "容量",
    collapseAction: "折りたたむ",
    configurationEmpty:
      "実行中のファクトリ定義に選択したリソースが含まれていません。",
    configurationErrorPrefix: "リソース定義を取得できません。",
    configurationLoading:
      "このリソースの現在のファクトリ定義を読み込んでいます。",
    editableConfigurationCapacityInvalid:
      "リソース容量は 1 より大きい整数である必要があります。",
    editableConfigurationCollapseActionLabel:
      "リソース設定エディターを折りたたむ",
    editableConfigurationContractInvalidPrefix: "リソース設定が無効です。",
    editableConfigurationDirtyStatus:
      "このリソースに未保存の変更があります。",
    editableConfigurationDraftNote:
      "保存するまで変更はこの編集セッション内にのみ保持されます。",
    editableConfigurationEmpty:
      "実行中のファクトリ定義に編集可能なリソース値がありません。",
    editableConfigurationErrorPrefix: "リソース設定を利用できません。",
    editableConfigurationExpandActionLabel: "リソース設定エディターを展開",
    editableConfigurationHeading: "リソース設定",
    editableConfigurationLoading: "編集可能なリソース設定を読み込んでいます。",
    editableConfigurationNameDuplicate: (resourceName) =>
      `実行中のファクトリ定義に "${resourceName}" という名前のリソースが既に存在します。`,
    editableConfigurationNameRequired:
      "保存する前にリソース名を入力してください。",
    editableConfigurationOverwriteWarning: (fields) =>
      `編集開始後に実行中のファクトリが変更されました。保存すると ${fields} の新しいサーバー値が上書きされます。`,
    editableConfigurationOverwriteWarningDetail:
      "保存前に最新のランタイム値を確認するか、この下書きで置き換える場合は編集を続けてください。",
    editableConfigurationSaveAction: "リソースを保存",
    editableConfigurationSaveBusyAction: "リソースを保存しています...",
    editableConfigurationSaveDisabledValidationDetail:
      "強調表示されたリソース項目が有効になるまで保存は無効です。",
    editableConfigurationSaveErrorPrefix: "保存に失敗しました。",
    editableConfigurationSaveFallbackError:
      "実行中のファクトリを保存できませんでした。",
    editableConfigurationSaveStaleVersionDetail:
      "最新の実行中ファクトリ値を再読み込みするか、エディター更新後にこの下書きで再試行してください。",
    editableConfigurationSaveSuccess: (resourceName) =>
      `実行中のファクトリを保存しました。${resourceName} が実行中のファクトリ定義で更新されました。`,
    editableConfigurationServerFieldChangedHint:
      "編集中に実行中のファクトリがこの項目を変更しました。最新のサーバー値を使うにはローカル変更を破棄してください。",
    editableConfigurationSharedImpactWarning: (
      resourceName,
      workerNames,
      workstationNames,
    ) =>
      `${resourceName} を保存すると、このリソースを参照するすべてのワーカーとワークステーション（ワーカー ${workerNames}、ワークステーション ${workstationNames}）が更新されます。`,
    editableConfigurationSharedImpactWarningDetail:
      "容量とリソースメタデータは、このリソース名が参照されるすべての場所に適用されます。",
    editableConfigurationValidationStatus:
      "保存する前に強調表示された項目を修正してください。",
    expandAction: "展開",
    loadPolicyFieldLabel: "ロードポリシー",
    modelFieldLabel: "モデル",
    nameFieldLabel: "名前",
    notConfiguredValue: "未設定",
    providerFieldLabel: "プロバイダー",
    referencingWorkersEmpty:
      "実行中のファクトリ定義でこのリソースを必要とするワーカーはありません。",
    referencingWorkersHeading: "参照ワーカー",
    referencingWorkstationsEmpty:
      "実行中のファクトリ定義でこのリソースを消費するワークステーションはありません。",
    referencingWorkstationsHeading: "参照ワークステーション",
    resetToLatestAction: "ローカル変更を破棄",
    summaryHeading: "概要",
    tokenCountFieldLabel: "利用可能トークン数",
    typeFieldLabel: "種別",
    unknownTypeValue: "不明な種別",
  },
  ko: {
    backendFieldLabel: "백엔드",
    capacityFieldLabel: "용량",
    collapseAction: "접기",
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 리소스가 포함되어 있지 않습니다.",
    configurationErrorPrefix: "리소스 정의를 사용할 수 없습니다.",
    configurationLoading: "이 리소스의 현재 팩토리 정의를 불러오는 중입니다.",
    editableConfigurationCapacityInvalid:
      "리소스 용량은 1보다 큰 정수여야 합니다.",
    editableConfigurationCollapseActionLabel: "리소스 구성 편집기 접기",
    editableConfigurationContractInvalidPrefix: "리소스 구성이 유효하지 않습니다.",
    editableConfigurationDirtyStatus:
      "이 리소스에 저장되지 않은 변경 사항이 있습니다.",
    editableConfigurationDraftNote:
      "저장하기 전까지 변경 사항은 이 편집 세션에만 유지됩니다.",
    editableConfigurationEmpty:
      "실행 중인 팩토리 정의에 편집 가능한 리소스 값이 없습니다.",
    editableConfigurationErrorPrefix: "리소스 구성을 사용할 수 없습니다.",
    editableConfigurationExpandActionLabel: "리소스 구성 편집기 펼치기",
    editableConfigurationHeading: "리소스 구성",
    editableConfigurationLoading: "편집 가능한 리소스 구성을 불러오는 중입니다.",
    editableConfigurationNameDuplicate: (resourceName) =>
      `실행 중인 팩토리 정의에 "${resourceName}"(이)라는 리소스가 이미 있습니다.`,
    editableConfigurationNameRequired:
      "저장하기 전에 리소스 이름을 입력하세요.",
    editableConfigurationOverwriteWarning: (fields) =>
      `편집을 시작한 뒤 실행 중인 팩토리가 변경되었습니다. 저장하면 ${fields}의 최신 서버 값이 덮어씌워집니다.`,
    editableConfigurationOverwriteWarningDetail:
      "저장하기 전에 최신 런타임 값을 검토하거나, 이 초안으로 대체하려면 편집을 계속하세요.",
    editableConfigurationSaveAction: "리소스 저장",
    editableConfigurationSaveBusyAction: "리소스 저장 중...",
    editableConfigurationSaveDisabledValidationDetail:
      "강조된 리소스 필드가 유효해질 때까지 저장이 비활성화됩니다.",
    editableConfigurationSaveErrorPrefix: "저장에 실패했습니다.",
    editableConfigurationSaveFallbackError:
      "실행 중인 팩토리를 저장할 수 없습니다.",
    editableConfigurationSaveStaleVersionDetail:
      "최신 실행 중 팩토리 값을 다시 불러오거나 편집기가 새로고침된 후 이 초안으로 다시 시도하세요.",
    editableConfigurationSaveSuccess: (resourceName) =>
      `실행 중인 팩토리가 저장되었습니다. ${resourceName}이(가) 실행 중인 팩토리 정의에서 업데이트되었습니다.`,
    editableConfigurationServerFieldChangedHint:
      "편집 중 실행 중인 팩토리가 이 필드를 변경했습니다. 최신 서버 값을 사용하려면 로컬 변경을 버리세요.",
    editableConfigurationSharedImpactWarning: (
      resourceName,
      workerNames,
      workstationNames,
    ) =>
      `${resourceName}을(를) 저장하면 이 리소스를 참조하는 모든 워커와 워크스테이션(워커 ${workerNames}, 워크스테이션 ${workstationNames})이 업데이트됩니다.`,
    editableConfigurationSharedImpactWarningDetail:
      "용량과 리소스 메타데이터는 이 리소스 이름이 참조되는 모든 위치에 적용됩니다.",
    editableConfigurationValidationStatus:
      "저장하기 전에 강조된 필드를 수정하세요.",
    expandAction: "펼치기",
    loadPolicyFieldLabel: "로드 정책",
    modelFieldLabel: "모델",
    nameFieldLabel: "이름",
    notConfiguredValue: "구성되지 않음",
    providerFieldLabel: "프로바이더",
    referencingWorkersEmpty:
      "실행 중인 팩토리 정의에서 이 리소스를 필요로 하는 워커가 없습니다.",
    referencingWorkersHeading: "참조 워커",
    referencingWorkstationsEmpty:
      "실행 중인 팩토리 정의에서 이 리소스를 소비하는 워크스테이션이 없습니다.",
    referencingWorkstationsHeading: "참조 워크스테이션",
    resetToLatestAction: "로컬 변경 버리기",
    summaryHeading: "요약",
    tokenCountFieldLabel: "사용 가능 토큰",
    typeFieldLabel: "유형",
    unknownTypeValue: "알 수 없는 유형",
  },
  "zh-CN": {
    backendFieldLabel: "后端",
    capacityFieldLabel: "容量",
    collapseAction: "收起",
    configurationEmpty: "当前运行中的工厂定义不包含所选资源。",
    configurationErrorPrefix: "资源定义不可用。",
    configurationLoading: "正在加载此资源的当前工厂定义。",
    editableConfigurationCapacityInvalid: "资源容量必须是大于零的整数。",
    editableConfigurationCollapseActionLabel: "收起资源配置编辑器",
    editableConfigurationContractInvalidPrefix: "资源配置无效。",
    editableConfigurationDirtyStatus: "此资源有未保存的更改。",
    editableConfigurationDraftNote: "保存前，更改仅保留在此编辑会话中。",
    editableConfigurationEmpty: "运行中的工厂定义没有可编辑的资源值。",
    editableConfigurationErrorPrefix: "无法加载资源配置。",
    editableConfigurationExpandActionLabel: "展开资源配置编辑器",
    editableConfigurationHeading: "资源配置",
    editableConfigurationLoading: "正在加载可编辑的资源配置。",
    editableConfigurationNameDuplicate: (resourceName) =>
      `运行中的工厂定义已存在名为 "${resourceName}" 的资源。`,
    editableConfigurationNameRequired: "保存此资源前请输入资源名称。",
    editableConfigurationOverwriteWarning: (fields) =>
      `开始编辑后运行中的工厂已发生变化。现在保存将覆盖 ${fields} 的较新服务器值。`,
    editableConfigurationOverwriteWarningDetail:
      "保存前请查看最新运行时值，或继续编辑以用此草稿替换它们。",
    editableConfigurationSaveAction: "保存资源",
    editableConfigurationSaveBusyAction: "正在保存资源...",
    editableConfigurationSaveDisabledValidationDetail:
      "高亮资源字段有效之前，保存保持禁用。",
    editableConfigurationSaveErrorPrefix: "保存失败。",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
    editableConfigurationSaveStaleVersionDetail:
      "重新加载最新的运行中工厂值，或在编辑器刷新后使用此草稿重试。",
    editableConfigurationSaveSuccess: (resourceName) =>
      `运行中的工厂已保存。${resourceName} 已在运行中的工厂定义中更新。`,
    editableConfigurationServerFieldChangedHint:
      "编辑期间运行中的工厂更改了此字段。丢弃本地更改以使用最新服务器值。",
    editableConfigurationSharedImpactWarning: (
      resourceName,
      workerNames,
      workstationNames,
    ) =>
      `保存 ${resourceName} 会更新引用此资源的所有工人和工位，包括工人 ${workerNames} 和工位 ${workstationNames}。`,
    editableConfigurationSharedImpactWarningDetail:
      "容量和资源元数据会应用到引用此资源名称的所有位置。",
    editableConfigurationValidationStatus: "保存此资源前请修正高亮字段。",
    expandAction: "展开",
    loadPolicyFieldLabel: "加载策略",
    modelFieldLabel: "模型",
    nameFieldLabel: "名称",
    notConfiguredValue: "未配置",
    providerFieldLabel: "提供商",
    referencingWorkersEmpty: "当前运行中的工厂定义没有工人需要此资源。",
    referencingWorkersHeading: "引用此资源的工人",
    referencingWorkstationsEmpty: "当前运行中的工厂定义没有工位消耗此资源。",
    referencingWorkstationsHeading: "引用此资源的工位",
    resetToLatestAction: "丢弃本地更改",
    summaryHeading: "摘要",
    tokenCountFieldLabel: "可用令牌数",
    typeFieldLabel: "类型",
    unknownTypeValue: "未知类型",
  },
} satisfies LocalizedMessages<ResourceDetailCatalogMessages>;

export function getResourceDetailMessages(
  locale?: string | null,
): ResourceDetailMessages {
  const enumMessages = getResourceDetailEnumMessages(locale);

  return {
    ...resolveLocalizedMessages(resourceDetailMessagesByLocale, locale),
    localizeResourceType: enumMessages.localizeResourceType,
  };
}
