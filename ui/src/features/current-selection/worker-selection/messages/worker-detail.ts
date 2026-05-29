import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { WorkerDetailMessages } from "./worker-detail-types";
import { getWorkerDetailEnumMessages } from "./worker-detail-enums";

type WorkerDetailCatalogMessages = Omit<
  WorkerDetailMessages,
  | "localizeExecutorProvider"
  | "localizeModelLocality"
  | "localizeModelProvider"
  | "localizeWorkerType"
>;

const workerDetailMessagesByLocale = {
  en: {
    argsFieldLabel: "Args",
    bodyFieldLabel: "Body",
    collapseAction: "Collapse",
    commandFieldLabel: "Command",
    configurationEmpty:
      "This running factory definition does not include the selected worker.",
    configurationErrorPrefix: "Worker definition unavailable.",
    configurationLoading: "Loading the current factory definition for this worker.",
    discardDraftAction: "Discard local changes",
    editableConfigurationCollapseActionLabel: "Collapse worker configuration editor",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable worker values.",
    editableConfigurationErrorPrefix: "Worker configuration unavailable.",
    editableConfigurationExpandActionLabel: "Expand worker configuration editor",
    editableConfigurationHeading: "Worker configuration",
    editableConfigurationArgsInvalid:
      "Each script argument must be a single non-empty line.",
    editableConfigurationBodyRequired:
      "Enter script body instructions before saving this worker.",
    editableConfigurationCommandRequired:
      "Enter a command before saving this worker.",
    editableConfigurationContractInvalidPrefix: "Worker configuration is invalid.",
    editableConfigurationDirtyStatus:
      "You have unsaved changes for this worker.",
    editableConfigurationDraftNote:
      "Changes stay local to this edit session until you save the running factory.",
    editableConfigurationLoading: "Loading editable worker configuration.",
    editableConfigurationModelProviderRequired:
      "Select a model provider before saving this worker.",
    editableConfigurationModelRequired:
      "Enter a model before saving this worker.",
    editableConfigurationProviderRequired:
      "Select a hosted provider before saving this worker.",
    editableConfigurationSaveAction: "Save worker",
    editableConfigurationSaveDisabledValidationDetail:
      "Save stays disabled until the highlighted worker fields are valid.",
    editableConfigurationScriptCommandOrBodyRequired:
      "Enter a command or script body before saving this worker.",
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `Saving ${workerName} updates every workstation that references this worker, including ${workstationNames}.`,
    editableConfigurationSharedImpactWarningDetail:
      "Provider, model, and worker instruction settings are worker-owned and apply to all listed workstations.",
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this worker.",
    executorProviderLabel: "Executor provider",
    expandAction: "Expand",
    modelLabel: "Model",
    modelLocalityLabel: "Model locality",
    modelProviderLabel: "Model provider",
    notConfiguredOptionLabel: "Not configured",
    notConfiguredValue: "Not configured",
    providerFieldLabel: "Hosted provider",
    referencingWorkstationsEmpty:
      "No workstations reference this worker in the running factory definition.",
    referencingWorkstationsHeading: "Referencing workstations",
    summaryHeading: "Summary",
    typeFieldLabel: "Worker type",
    typeLabel: "Worker type",
    unknownTypeValue: "Unknown",
  },
  ja: {
    argsFieldLabel: "引数",
    bodyFieldLabel: "本文",
    collapseAction: "折りたたむ",
    commandFieldLabel: "コマンド",
    configurationEmpty:
      "実行中のファクトリ定義に選択したワーカーが含まれていません。",
    configurationErrorPrefix: "ワーカー定義を取得できません。",
    configurationLoading:
      "このワーカーの現在のファクトリ定義を読み込んでいます。",
    discardDraftAction: "ローカル変更を破棄",
    editableConfigurationCollapseActionLabel: "ワーカー設定エディターを折りたたむ",
    editableConfigurationEmpty:
      "実行中のファクトリ定義に編集可能なワーカー値がありません。",
    editableConfigurationErrorPrefix: "ワーカー設定を利用できません。",
    editableConfigurationExpandActionLabel: "ワーカー設定エディターを展開",
    editableConfigurationHeading: "ワーカー設定",
    editableConfigurationArgsInvalid:
      "各スクリプト引数は空でない 1 行である必要があります。",
    editableConfigurationBodyRequired:
      "このワーカーを保存する前にスクリプト本文を入力してください。",
    editableConfigurationCommandRequired:
      "このワーカーを保存する前にコマンドを入力してください。",
    editableConfigurationContractInvalidPrefix: "ワーカー設定が無効です。",
    editableConfigurationDirtyStatus: "このワーカーに未保存の変更があります。",
    editableConfigurationDraftNote:
      "保存するまで変更はこの編集セッション内にのみ保持されます。",
    editableConfigurationLoading: "編集可能なワーカー設定を読み込んでいます。",
    editableConfigurationModelProviderRequired:
      "このワーカーを保存する前にモデルプロバイダーを選択してください。",
    editableConfigurationModelRequired:
      "このワーカーを保存する前にモデルを入力してください。",
    editableConfigurationProviderRequired:
      "このワーカーを保存する前にホスト型プロバイダーを選択してください。",
    editableConfigurationSaveAction: "ワーカーを保存",
    editableConfigurationSaveDisabledValidationDetail:
      "ハイライトされたワーカー項目が有効になるまで保存は無効のままです。",
    editableConfigurationScriptCommandOrBodyRequired:
      "このワーカーを保存する前にコマンドまたはスクリプト本文を入力してください。",
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `${workerName} を保存すると、このワーカーを参照するすべてのワークステーション（${workstationNames} を含む）が更新されます。`,
    editableConfigurationSharedImpactWarningDetail:
      "プロバイダー、モデル、ワーカー指示はワーカー所有であり、一覧のすべてのワークステーションに適用されます。",
    editableConfigurationValidationStatus:
      "このワーカーを保存する前にハイライトされた項目を修正してください。",
    executorProviderLabel: "実行プロバイダー",
    expandAction: "展開",
    modelLabel: "モデル",
    modelLocalityLabel: "モデルローカリティ",
    modelProviderLabel: "モデルプロバイダー",
    notConfiguredOptionLabel: "未設定",
    notConfiguredValue: "未設定",
    providerFieldLabel: "ホスト型プロバイダー",
    referencingWorkstationsEmpty:
      "実行中のファクトリ定義でこのワーカーを参照するワークステーションはありません。",
    referencingWorkstationsHeading: "参照ワークステーション",
    summaryHeading: "概要",
    typeFieldLabel: "ワーカー種別",
    typeLabel: "ワーカー種別",
    unknownTypeValue: "不明",
  },
  ko: {
    argsFieldLabel: "인수",
    bodyFieldLabel: "본문",
    collapseAction: "접기",
    commandFieldLabel: "명령",
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 워커가 포함되어 있지 않습니다.",
    configurationErrorPrefix: "워커 정의를 사용할 수 없습니다.",
    configurationLoading:
      "이 워커의 현재 팩토리 정의를 불러오는 중입니다.",
    discardDraftAction: "로컬 변경 사항 취소",
    editableConfigurationCollapseActionLabel: "워커 구성 편집기 접기",
    editableConfigurationEmpty:
      "실행 중인 팩토리 정의에 편집 가능한 워커 값이 없습니다.",
    editableConfigurationErrorPrefix: "워커 구성을 사용할 수 없습니다.",
    editableConfigurationExpandActionLabel: "워커 구성 편집기 펼치기",
    editableConfigurationHeading: "워커 구성",
    editableConfigurationArgsInvalid:
      "각 스크립트 인수는 비어 있지 않은 한 줄이어야 합니다.",
    editableConfigurationBodyRequired:
      "이 워커를 저장하기 전에 스크립트 본문을 입력하세요.",
    editableConfigurationCommandRequired:
      "이 워커를 저장하기 전에 명령을 입력하세요.",
    editableConfigurationContractInvalidPrefix: "워커 구성이 유효하지 않습니다.",
    editableConfigurationDirtyStatus: "이 워커에 저장되지 않은 변경 사항이 있습니다.",
    editableConfigurationDraftNote:
      "저장할 때까지 변경 사항은 이 편집 세션에만 유지됩니다.",
    editableConfigurationLoading: "편집 가능한 워커 구성을 불러오는 중입니다.",
    editableConfigurationModelProviderRequired:
      "이 워커를 저장하기 전에 모델 제공자를 선택하세요.",
    editableConfigurationModelRequired:
      "이 워커를 저장하기 전에 모델을 입력하세요.",
    editableConfigurationProviderRequired:
      "이 워커를 저장하기 전에 호스티드 제공자를 선택하세요.",
    editableConfigurationSaveAction: "워커 저장",
    editableConfigurationSaveDisabledValidationDetail:
      "강조된 워커 필드가 유효해질 때까지 저장은 비활성화됩니다.",
    editableConfigurationScriptCommandOrBodyRequired:
      "이 워커를 저장하기 전에 명령 또는 스크립트 본문을 입력하세요.",
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `${workerName} 저장은 이 워커를 참조하는 모든 워크스테이션(${workstationNames} 포함)을 업데이트합니다.`,
    editableConfigurationSharedImpactWarningDetail:
      "제공자, 모델, 워커 지침은 워커 소유이며 나열된 모든 워크스테이션에 적용됩니다.",
    editableConfigurationValidationStatus:
      "이 워커를 저장하기 전에 강조된 필드를 해결하세요.",
    executorProviderLabel: "실행자 제공자",
    expandAction: "펼치기",
    modelLabel: "모델",
    modelLocalityLabel: "모델 지역성",
    modelProviderLabel: "모델 제공자",
    notConfiguredOptionLabel: "구성되지 않음",
    notConfiguredValue: "구성되지 않음",
    providerFieldLabel: "호스티드 제공자",
    referencingWorkstationsEmpty:
      "실행 중인 팩토리 정의에서 이 워커를 참조하는 워크스테이션이 없습니다.",
    referencingWorkstationsHeading: "참조 워크스테이션",
    summaryHeading: "요약",
    typeFieldLabel: "워커 유형",
    typeLabel: "워커 유형",
    unknownTypeValue: "알 수 없음",
  },
  "zh-CN": {
    argsFieldLabel: "参数",
    bodyFieldLabel: "正文",
    collapseAction: "收起",
    commandFieldLabel: "命令",
    configurationEmpty: "运行中的工厂定义不包含所选 worker。",
    configurationErrorPrefix: "无法加载 worker 定义。",
    configurationLoading: "正在加载此 worker 的当前工厂定义。",
    discardDraftAction: "放弃本地更改",
    editableConfigurationCollapseActionLabel: "收起 worker 配置编辑器",
    editableConfigurationEmpty: "运行中的工厂定义没有可编辑的 worker 值。",
    editableConfigurationErrorPrefix: "无法加载 worker 配置。",
    editableConfigurationExpandActionLabel: "展开 worker 配置编辑器",
    editableConfigurationHeading: "Worker 配置",
    editableConfigurationArgsInvalid: "每个脚本参数必须是非空的一行。",
    editableConfigurationBodyRequired: "保存此 worker 前请输入脚本正文。",
    editableConfigurationCommandRequired: "保存此 worker 前请输入命令。",
    editableConfigurationContractInvalidPrefix: "Worker 配置无效。",
    editableConfigurationDirtyStatus: "此 worker 有未保存的更改。",
    editableConfigurationDraftNote: "保存前，更改仅保留在此编辑会话中。",
    editableConfigurationLoading: "正在加载可编辑的 worker 配置。",
    editableConfigurationModelProviderRequired: "保存此 worker 前请选择模型 provider。",
    editableConfigurationModelRequired: "保存此 worker 前请输入模型。",
    editableConfigurationProviderRequired: "保存此 worker 前请选择托管 provider。",
    editableConfigurationSaveAction: "保存 worker",
    editableConfigurationSaveDisabledValidationDetail:
      "高亮字段有效前，保存将保持禁用。",
    editableConfigurationScriptCommandOrBodyRequired:
      "保存此 worker 前请输入命令或脚本正文。",
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `保存 ${workerName} 会更新引用此 worker 的所有 workstation，包括 ${workstationNames}。`,
    editableConfigurationSharedImpactWarningDetail:
      "Provider、模型和 worker 指令归 worker 所有，并应用于列出的所有 workstation。",
    editableConfigurationValidationStatus: "保存此 worker 前请修正高亮字段。",
    executorProviderLabel: "执行器 provider",
    expandAction: "展开",
    modelLabel: "模型",
    modelLocalityLabel: "模型位置",
    modelProviderLabel: "模型 provider",
    notConfiguredOptionLabel: "未配置",
    notConfiguredValue: "未配置",
    providerFieldLabel: "托管 provider",
    referencingWorkstationsEmpty:
      "运行中的工厂定义中没有 workstation 引用此 worker。",
    referencingWorkstationsHeading: "引用 workstation",
    summaryHeading: "摘要",
    typeFieldLabel: "Worker 类型",
    typeLabel: "Worker 类型",
    unknownTypeValue: "未知",
  },
} satisfies LocalizedMessages<WorkerDetailCatalogMessages>;

export { workerDetailMessagesByLocale };

export function getWorkerDetailMessages(
  locale?: string | null,
): WorkerDetailMessages {
  const enumMessages = getWorkerDetailEnumMessages(locale);
  const catalog = resolveLocalizedMessages(workerDetailMessagesByLocale, locale);

  return {
    ...catalog,
    ...enumMessages,
  };
}
