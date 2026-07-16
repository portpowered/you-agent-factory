// biome-ignore-all lint/style/noExcessiveLinesPerFile: feature-local locale catalogs keep required language coverage in one typed asset set.
import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import { getWorkerDetailEnumMessages } from "./worker-detail-enums";
import type { WorkerDetailMessages } from "./worker-detail-types";

type WorkerDetailCatalogMessages = Omit<
  WorkerDetailMessages,
  | "localizeExecutorProvider"
  | "localizeModelLocality"
  | "localizeModelProvider"
  | "localizeTimeoutUnit"
  | "localizeWorkerType"
>;

const workerDetailMessagesByLocale = {
  en: {
    argsFieldLabel: "Args",
    authSecretRefFieldHelp:
      "Reference name for the Linear API secret in your environment or secrets file (for example secrets/linear-api-key). The website never stores or displays the secret value.",
    authSecretRefFieldLabel: "Secret reference",
    bodyFieldLabel: "Body",
    collapseAction: "Collapse",
    commandFieldLabel: "Command",
    configurationEmpty:
      "This running factory definition does not include the selected worker.",
    configurationErrorPrefix: "Worker definition unavailable.",
    configurationLoading:
      "Loading the current factory definition for this worker.",
    discardDraftAction: "Discard local changes",
    editableConfigurationCollapseActionLabel:
      "Collapse worker configuration editor",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable worker values.",
    editableConfigurationErrorPrefix: "Worker configuration unavailable.",
    editableConfigurationExpandActionLabel:
      "Expand worker configuration editor",
    editableConfigurationHeading: "Worker configuration",
    editableConfigurationArgsInvalid:
      "Each script argument must be a single non-empty line.",
    editableConfigurationAuthSecretRefRequired:
      "Enter a secret reference before saving this hosted Linear worker.",
    editableConfigurationBodyRequired:
      "Enter script body instructions before saving this worker.",
    editableConfigurationCommandRequired:
      "Enter a command before saving this worker.",
    editableConfigurationContractInvalidPrefix:
      "Worker configuration is invalid.",
    editableConfigurationOverwriteWarning: (fields) =>
      `The running factory changed after you started editing. Saving now will overwrite newer server values for ${fields}.`,
    editableConfigurationOverwriteWarningDetail:
      "Review the latest runtime values before saving, or keep editing if this draft should replace them.",
    editableConfigurationServerFieldChangedHint:
      "The running factory changed this field while you were editing. Discard local changes to use the latest server-backed value.",
    editableConfigurationLoading: "Loading editable worker configuration.",
    editableConfigurationModelProviderRequired:
      "Select a model provider before saving this worker.",
    editableConfigurationModelRequired:
      "Enter a model before saving this worker.",
    editableConfigurationNameDuplicate: (workerName) =>
      `A worker named "${workerName}" already exists in the running factory definition.`,
    editableConfigurationNameRequired:
      "Enter a worker name before saving this worker.",
    editableConfigurationProviderRequired:
      "Select a hosted provider before saving this worker.",
    editableConfigurationSaveAction: "Save worker",
    editableConfigurationSaveBusyAction: "Saving worker...",
    editableConfigurationSaveDisabledValidationDetail:
      "Save stays disabled until the highlighted worker fields are valid.",
    editableConfigurationSaveErrorPrefix: "Saving failed.",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
    editableConfigurationSaveStaleVersionDetail:
      "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
    editableConfigurationSaveSuccess: (workerName) =>
      `Running factory saved. ${workerName} was updated in the running factory definition.`,
    editableConfigurationScriptCommandOrBodyRequired:
      "Enter a command or script body before saving this worker.",
    editableConfigurationTimeoutInvalid: (value) =>
      `timeout must be a positive duration such as 30s, 5m, or 1h, got ${JSON.stringify(value)}`,
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `Saving ${workerName} updates workstations ${workstationNames}.`,
    editableConfigurationSharedImpactWarningDetail:
      "Worker-owned settings apply to all listed workstations.",
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this worker.",
    executorProviderLabel: "Executor provider",
    expandAction: "Expand",
    editableConfigurationLinearClaimAssigneeFieldRequired:
      "Enter a claim assignee field before saving this hosted Linear worker.",
    editableConfigurationLinearMappingStateRequired:
      "Enter a mapping state before saving this hosted Linear worker.",
    editableConfigurationLinearMappingWorkTypeRequired:
      "Enter a mapping work type before saving this hosted Linear worker.",
    linearClaimAssigneeFieldFieldHelp:
      "Optional. Linear field path used when claiming issues (for example assignee.email). Required when claim is already configured on this worker.",
    linearClaimAssigneeFieldLabel: "Claim assignee field",
    linearMappingStateFieldHelp:
      "Initial work state assigned to submissions created from polled Linear issues.",
    linearMappingStateFieldLabel: "Mapping state",
    linearMappingWorkTypeFieldHelp:
      "Work type assigned to submissions created from polled Linear issues.",
    linearMappingWorkTypeFieldLabel: "Mapping work type",
    linearPollIntervalFieldHelp:
      "Optional. How often the poller checks Linear for new issues (for example 30s or 5m).",
    linearPollIntervalFieldLabel: "Poll interval",
    linearStateIdsFieldHelp:
      "Optional. One Linear workflow state ID per line to filter polled issues.",
    linearStateIdsFieldLabel: "State IDs",
    linearTeamIdsFieldHelp:
      "Optional. One Linear team ID per line to limit polling scope.",
    linearTeamIdsFieldLabel: "Team IDs",
    modelFieldHelp: "Blank uses the provider default model.",
    modelLabel: "Model",
    modelLocalityLabel: "Model locality",
    modelProviderFieldHelp:
      "Required for model workers; sets routing and default model.",
    modelProviderLabel: "Model provider",
    nameFieldLabel: "Worker name",
    notConfiguredOptionLabel: "Not configured",
    notConfiguredValue: "Not configured",
    providerFieldLabel: "Hosted provider",
    referencingWorkstationsEmpty:
      "No workstations reference this worker in the running factory definition.",
    referencingWorkstationsHeading: "Referencing workstations",
    skipPermissionsFieldHelp:
      "When enabled, supported model providers can bypass permission prompts during execution.",
    skipPermissionsFieldLabel: "Bypass provider permissions",
    stopTokenFieldHelp:
      "Optional. Worker-owned marker that treats model-oriented output as complete when it appears. This is separate from workstation stop words.",
    stopTokenFieldLabel: "Stop token",
    timeoutFieldHelp:
      "Optional. Limits how long a worker run may execute (for example 30s, 5m, or 1h).",
    timeoutFieldLabel: "Execution timeout",
    summaryHeading: "Summary",
    typeFieldLabel: "Worker type",
    typeLabel: "Worker type",
    unknownTypeValue: "Unknown",
  },
  ja: {
    argsFieldLabel: "引数",
    authSecretRefFieldHelp:
      "環境またはシークレットファイル内の LINEAR API シークレット参照名です（例: secrets/linear-api-key）。ウェブサイトはシークレット値を保存または表示しません。",
    authSecretRefFieldLabel: "シークレット参照",
    bodyFieldLabel: "本文",
    collapseAction: "折りたたむ",
    commandFieldLabel: "コマンド",
    configurationEmpty:
      "実行中のファクトリ定義に選択したワーカーが含まれていません。",
    configurationErrorPrefix: "ワーカー定義を取得できません。",
    configurationLoading:
      "このワーカーの現在のファクトリ定義を読み込んでいます。",
    discardDraftAction: "ローカル変更を破棄",
    editableConfigurationCollapseActionLabel:
      "ワーカー設定エディターを折りたたむ",
    editableConfigurationEmpty:
      "実行中のファクトリ定義に編集可能なワーカー値がありません。",
    editableConfigurationErrorPrefix: "ワーカー設定を利用できません。",
    editableConfigurationExpandActionLabel: "ワーカー設定エディターを展開",
    editableConfigurationHeading: "ワーカー設定",
    editableConfigurationArgsInvalid:
      "各スクリプト引数は空でない 1 行である必要があります。",
    editableConfigurationAuthSecretRefRequired:
      "このホスト型 LINEAR ワーカーを保存する前にシークレット参照を入力してください。",
    editableConfigurationBodyRequired:
      "このワーカーを保存する前にスクリプト本文を入力してください。",
    editableConfigurationCommandRequired:
      "このワーカーを保存する前にコマンドを入力してください。",
    editableConfigurationContractInvalidPrefix: "ワーカー設定が無効です。",
    editableConfigurationOverwriteWarning: (fields) =>
      `編集開始後に実行中のファクトリが変更されました。保存すると ${fields} の新しいサーバー値を上書きします。`,
    editableConfigurationOverwriteWarningDetail:
      "保存前に最新のランタイム値を確認するか、このドラフトで置き換える必要がある場合は編集を続けてください。",
    editableConfigurationServerFieldChangedHint:
      "編集中に実行中のファクトリがこの項目を変更しました。ローカル変更を破棄すると最新のサーバー値が使われます。",
    editableConfigurationLoading: "編集可能なワーカー設定を読み込んでいます。",
    editableConfigurationModelProviderRequired:
      "このワーカーを保存する前にモデルプロバイダーを選択してください。",
    editableConfigurationModelRequired:
      "このワーカーを保存する前にモデルを入力してください。",
    editableConfigurationNameDuplicate: (workerName) =>
      `実行中のファクトリ定義には "${workerName}" という名前のワーカーがすでに存在します。`,
    editableConfigurationNameRequired:
      "このワーカーを保存する前にワーカー名を入力してください。",
    editableConfigurationProviderRequired:
      "このワーカーを保存する前にホスト型プロバイダーを選択してください。",
    editableConfigurationSaveAction: "ワーカーを保存",
    editableConfigurationSaveBusyAction: "ワーカーを保存しています...",
    editableConfigurationSaveDisabledValidationDetail:
      "ハイライトされたワーカー項目が有効になるまで保存は無効のままです。",
    editableConfigurationSaveErrorPrefix: "保存に失敗しました。",
    editableConfigurationSaveFallbackError:
      "実行中のファクトリを保存できませんでした。",
    editableConfigurationSaveStaleVersionDetail:
      "最新の実行中ファクトリ値を再読み込みするか、このドラフトを保持したままエディター更新後に再試行してください。",
    editableConfigurationSaveSuccess: (workerName) =>
      `実行中のファクトリを保存しました。${workerName} が実行中のファクトリ定義で更新されました。`,
    editableConfigurationScriptCommandOrBodyRequired:
      "このワーカーを保存する前にコマンドまたはスクリプト本文を入力してください。",
    editableConfigurationTimeoutInvalid: (value) =>
      `timeout は 30s、5m、1h などの正の時間である必要があります。入力値: ${JSON.stringify(value)}`,
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `${workerName} を保存すると、ワークステーション ${workstationNames} が更新されます。`,
    editableConfigurationSharedImpactWarningDetail:
      "ワーカー所有の設定は一覧のすべてのワークステーションに適用されます。",
    editableConfigurationValidationStatus:
      "このワーカーを保存する前にハイライトされた項目を修正してください。",
    executorProviderLabel: "実行プロバイダー",
    expandAction: "展開",
    editableConfigurationLinearClaimAssigneeFieldRequired:
      "このホスト型 LINEAR ワーカーを保存する前にクレーム担当者フィールドを入力してください。",
    editableConfigurationLinearMappingStateRequired:
      "このホスト型 LINEAR ワーカーを保存する前にマッピング状態を入力してください。",
    editableConfigurationLinearMappingWorkTypeRequired:
      "このホスト型 LINEAR ワーカーを保存する前にマッピング作業種別を入力してください。",
    linearClaimAssigneeFieldFieldHelp:
      "任意。課題をクレームするときに使う LINEAR フィールドパスです（例: assignee.email）。このワーカーにクレームが既に設定されている場合は必須です。",
    linearClaimAssigneeFieldLabel: "クレーム担当者フィールド",
    linearMappingStateFieldHelp:
      "ポーリングした LINEAR 課題から作成する submission に割り当てる初期作業状態です。",
    linearMappingStateFieldLabel: "マッピング状態",
    linearMappingWorkTypeFieldHelp:
      "ポーリングした LINEAR 課題から作成する submission に割り当てる作業種別です。",
    linearMappingWorkTypeFieldLabel: "マッピング作業種別",
    linearPollIntervalFieldHelp:
      "任意。ポーラーが LINEAR の新規課題を確認する間隔です（例: 30s または 5m）。",
    linearPollIntervalFieldLabel: "ポーリング間隔",
    linearStateIdsFieldHelp:
      "任意。ポーリング対象を絞り込む LINEAR ワークフロー状態 ID を 1 行に 1 つ入力します。",
    linearStateIdsFieldLabel: "状態 ID",
    linearTeamIdsFieldHelp:
      "任意。ポーリング範囲を限定する LINEAR チーム ID を 1 行に 1 つ入力します。",
    linearTeamIdsFieldLabel: "チーム ID",
    modelFieldHelp:
      "任意です。空のままにするとプロバイダーの既定モデル識別子が使われます。",
    modelLabel: "モデル",
    modelLocalityLabel: "モデルローカリティ",
    modelProviderFieldHelp:
      "モデルワーカーでは必須です。プロバイダーがルーティングと既定モデル動作を決定します。",
    modelProviderLabel: "モデルプロバイダー",
    nameFieldLabel: "ワーカー名",
    notConfiguredOptionLabel: "未設定",
    notConfiguredValue: "未設定",
    providerFieldLabel: "ホスト型プロバイダー",
    referencingWorkstationsEmpty:
      "実行中のファクトリ定義でこのワーカーを参照するワークステーションはありません。",
    referencingWorkstationsHeading: "参照ワークステーション",
    skipPermissionsFieldHelp:
      "有効にすると、対応するモデルプロバイダーが実行中の権限プロンプトを省略できます。",
    skipPermissionsFieldLabel: "プロバイダー権限を省略",
    stopTokenFieldHelp:
      "任意。ワーカー所有のマーカーで、表示されたときにモデル指向の出力を完了とみなします。ワークステーションのストップワードとは別です。",
    stopTokenFieldLabel: "ストップトークン",
    timeoutFieldHelp:
      "任意。ワーカー実行の上限時間を設定します（例: 30s、5m、1h）。",
    timeoutFieldLabel: "実行タイムアウト",
    summaryHeading: "概要",
    typeFieldLabel: "ワーカー種別",
    typeLabel: "ワーカー種別",
    unknownTypeValue: "不明",
  },
  ko: {
    argsFieldLabel: "인수",
    authSecretRefFieldHelp:
      "환경 또는 시크릿 파일에 있는 LINEAR API 시크릿 참조 이름입니다(예: secrets/linear-api-key). 웹사이트는 시크릿 값을 저장하거나 표시하지 않습니다.",
    authSecretRefFieldLabel: "시크릿 참조",
    bodyFieldLabel: "본문",
    collapseAction: "접기",
    commandFieldLabel: "명령",
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 워커가 포함되어 있지 않습니다.",
    configurationErrorPrefix: "워커 정의를 사용할 수 없습니다.",
    configurationLoading: "이 워커의 현재 팩토리 정의를 불러오는 중입니다.",
    discardDraftAction: "로컬 변경 사항 취소",
    editableConfigurationCollapseActionLabel: "워커 구성 편집기 접기",
    editableConfigurationEmpty:
      "실행 중인 팩토리 정의에 편집 가능한 워커 값이 없습니다.",
    editableConfigurationErrorPrefix: "워커 구성을 사용할 수 없습니다.",
    editableConfigurationExpandActionLabel: "워커 구성 편집기 펼치기",
    editableConfigurationHeading: "워커 구성",
    editableConfigurationArgsInvalid:
      "각 스크립트 인수는 비어 있지 않은 한 줄이어야 합니다.",
    editableConfigurationAuthSecretRefRequired:
      "이 호스티드 LINEAR 워커를 저장하기 전에 시크릿 참조를 입력하세요.",
    editableConfigurationBodyRequired:
      "이 워커를 저장하기 전에 스크립트 본문을 입력하세요.",
    editableConfigurationCommandRequired:
      "이 워커를 저장하기 전에 명령을 입력하세요.",
    editableConfigurationContractInvalidPrefix:
      "워커 구성이 유효하지 않습니다.",
    editableConfigurationOverwriteWarning: (fields) =>
      `편집을 시작한 뒤 실행 중인 팩토리가 변경되었습니다. 저장하면 ${fields}의 최신 서버 값을 덮어씁니다.`,
    editableConfigurationOverwriteWarningDetail:
      "저장하기 전에 최신 런타임 값을 검토하거나, 이 초안으로 대체해야 한다면 편집을 계속하세요.",
    editableConfigurationServerFieldChangedHint:
      "편집 중에 실행 중인 팩토리가 이 필드를 변경했습니다. 로컬 변경 사항을 취소하면 최신 서버 값이 사용됩니다.",
    editableConfigurationLoading: "편집 가능한 워커 구성을 불러오는 중입니다.",
    editableConfigurationModelProviderRequired:
      "이 워커를 저장하기 전에 모델 제공자를 선택하세요.",
    editableConfigurationModelRequired:
      "이 워커를 저장하기 전에 모델을 입력하세요.",
    editableConfigurationNameDuplicate: (workerName) =>
      `실행 중인 팩토리 정의에 "${workerName}"(이)라는 이름의 워커가 이미 있습니다.`,
    editableConfigurationNameRequired:
      "이 워커를 저장하기 전에 워커 이름을 입력하세요.",
    editableConfigurationProviderRequired:
      "이 워커를 저장하기 전에 호스티드 제공자를 선택하세요.",
    editableConfigurationSaveAction: "워커 저장",
    editableConfigurationSaveBusyAction: "워커 저장 중...",
    editableConfigurationSaveDisabledValidationDetail:
      "강조된 워커 필드가 유효해질 때까지 저장은 비활성화됩니다.",
    editableConfigurationSaveErrorPrefix: "저장에 실패했습니다.",
    editableConfigurationSaveFallbackError:
      "실행 중인 팩토리를 저장할 수 없습니다.",
    editableConfigurationSaveStaleVersionDetail:
      "최신 실행 중 팩토리 값을 다시 불러오거나 이 초안을 유지한 채 편집기가 새로고침된 후 다시 시도하세요.",
    editableConfigurationSaveSuccess: (workerName) =>
      `실행 중인 팩토리가 저장되었습니다. ${workerName} 이(가) 실행 중인 팩토리 정의에서 업데이트되었습니다.`,
    editableConfigurationScriptCommandOrBodyRequired:
      "이 워커를 저장하기 전에 명령 또는 스크립트 본문을 입력하세요.",
    editableConfigurationTimeoutInvalid: (value) =>
      `timeout은 30s, 5m, 1h처럼 양의 duration이어야 합니다. 입력값: ${JSON.stringify(value)}`,
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `${workerName} 저장은 워크스테이션 ${workstationNames}을(를) 업데이트합니다.`,
    editableConfigurationSharedImpactWarningDetail:
      "워커 소유 설정은 나열된 모든 워크스테이션에 적용됩니다.",
    editableConfigurationValidationStatus:
      "이 워커를 저장하기 전에 강조된 필드를 해결하세요.",
    executorProviderLabel: "실행자 제공자",
    expandAction: "펼치기",
    editableConfigurationLinearClaimAssigneeFieldRequired:
      "이 호스티드 LINEAR 워커를 저장하기 전에 클레임 담당자 필드를 입력하세요.",
    editableConfigurationLinearMappingStateRequired:
      "이 호스티드 LINEAR 워커를 저장하기 전에 매핑 상태를 입력하세요.",
    editableConfigurationLinearMappingWorkTypeRequired:
      "이 호스티드 LINEAR 워커를 저장하기 전에 매핑 작업 유형을 입력하세요.",
    linearClaimAssigneeFieldFieldHelp:
      "선택 사항. 이슈를 클레임할 때 사용하는 LINEAR 필드 경로입니다(예: assignee.email). 이 워커에 클레임이 이미 구성되어 있으면 필수입니다.",
    linearClaimAssigneeFieldLabel: "클레임 담당자 필드",
    linearMappingStateFieldHelp:
      "폴링한 LINEAR 이슈에서 생성되는 submission에 할당할 초기 작업 상태입니다.",
    linearMappingStateFieldLabel: "매핑 상태",
    linearMappingWorkTypeFieldHelp:
      "폴링한 LINEAR 이슈에서 생성되는 submission에 할당할 작업 유형입니다.",
    linearMappingWorkTypeFieldLabel: "매핑 작업 유형",
    linearPollIntervalFieldHelp:
      "선택 사항. 폴러가 LINEAR의 새 이슈를 확인하는 간격입니다(예: 30s 또는 5m).",
    linearPollIntervalFieldLabel: "폴링 간격",
    linearStateIdsFieldHelp:
      "선택 사항. 폴링 대상을 필터링할 LINEAR 워크플로 상태 ID를 한 줄에 하나씩 입력합니다.",
    linearStateIdsFieldLabel: "상태 ID",
    linearTeamIdsFieldHelp:
      "선택 사항. 폴링 범위를 제한할 LINEAR 팀 ID를 한 줄에 하나씩 입력합니다.",
    linearTeamIdsFieldLabel: "팀 ID",
    modelFieldHelp:
      "선택 사항입니다. 비워 두면 제공자 기본 모델 식별자가 사용됩니다.",
    modelLabel: "모델",
    modelLocalityLabel: "모델 지역성",
    modelProviderFieldHelp:
      "모델 워커에 필수입니다. 제공자가 라우팅과 기본 모델 동작을 선택합니다.",
    modelProviderLabel: "모델 제공자",
    nameFieldLabel: "워커 이름",
    notConfiguredOptionLabel: "구성되지 않음",
    notConfiguredValue: "구성되지 않음",
    providerFieldLabel: "호스티드 제공자",
    referencingWorkstationsEmpty:
      "실행 중인 팩토리 정의에서 이 워커를 참조하는 워크스테이션이 없습니다.",
    referencingWorkstationsHeading: "참조 워크스테이션",
    skipPermissionsFieldHelp:
      "활성화하면 지원되는 모델 제공자가 실행 중 권한 프롬프트를 건너뛸 수 있습니다.",
    skipPermissionsFieldLabel: "제공자 권한 건너뛰기",
    stopTokenFieldHelp:
      "선택 사항. 워커 소유 마커로, 표시되면 모델 지향 출력을 완료로 처리합니다. 워크스테이션 stop word와는 별개입니다.",
    stopTokenFieldLabel: "중지 토큰",
    timeoutFieldHelp:
      "선택 사항. 워커 실행 시간 상한을 설정합니다(예: 30s, 5m, 1h).",
    timeoutFieldLabel: "실행 타임아웃",
    summaryHeading: "요약",
    typeFieldLabel: "워커 유형",
    typeLabel: "워커 유형",
    unknownTypeValue: "알 수 없음",
  },
  "zh-CN": {
    argsFieldLabel: "参数",
    authSecretRefFieldHelp:
      "环境或密钥文件中的 LINEAR API 密钥引用名称（例如 secrets/linear-api-key）。网站不会存储或显示密钥值。",
    authSecretRefFieldLabel: "密钥引用",
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
    editableConfigurationAuthSecretRefRequired:
      "保存此托管 LINEAR worker 前请输入密钥引用。",
    editableConfigurationBodyRequired: "保存此 worker 前请输入脚本正文。",
    editableConfigurationCommandRequired: "保存此 worker 前请输入命令。",
    editableConfigurationContractInvalidPrefix: "Worker 配置无效。",
    editableConfigurationOverwriteWarning: (fields) =>
      `开始编辑后运行中的工厂已更改。保存将覆盖 ${fields} 的较新服务器值。`,
    editableConfigurationOverwriteWarningDetail:
      "保存前请查看最新的运行时值，或继续编辑以用此草稿替换它们。",
    editableConfigurationServerFieldChangedHint:
      "编辑期间运行中的工厂更改了此字段。放弃本地更改将使用最新的服务器值。",
    editableConfigurationLoading: "正在加载可编辑的 worker 配置。",
    editableConfigurationModelProviderRequired:
      "保存此 worker 前请选择模型 provider。",
    editableConfigurationModelRequired: "保存此 worker 前请输入模型。",
    editableConfigurationNameDuplicate: (workerName) =>
      `运行中的工厂定义已存在名为 "${workerName}" 的 worker。`,
    editableConfigurationNameRequired: "保存此 worker 前请输入 worker 名称。",
    editableConfigurationProviderRequired:
      "保存此 worker 前请选择托管 provider。",
    editableConfigurationSaveAction: "保存 worker",
    editableConfigurationSaveBusyAction: "正在保存 worker...",
    editableConfigurationSaveDisabledValidationDetail:
      "高亮字段有效前，保存将保持禁用。",
    editableConfigurationSaveErrorPrefix: "保存失败。",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
    editableConfigurationSaveStaleVersionDetail:
      "请重新加载最新的运行中工厂值，或保留此草稿并在编辑器刷新后重试。",
    editableConfigurationSaveSuccess: (workerName) =>
      `运行中的工厂已保存。${workerName} 已在运行中的工厂定义中更新。`,
    editableConfigurationScriptCommandOrBodyRequired:
      "保存此 worker 前请输入命令或脚本正文。",
    editableConfigurationTimeoutInvalid: (value) =>
      `timeout 必须是正数时长（例如 30s、5m 或 1h），当前为 ${JSON.stringify(value)}`,
    editableConfigurationSharedImpactWarning: (workerName, workstationNames) =>
      `保存 ${workerName} 会更新 workstation ${workstationNames}。`,
    editableConfigurationSharedImpactWarningDetail:
      "Worker 拥有的设置会应用于列出的所有 workstation。",
    editableConfigurationValidationStatus: "保存此 worker 前请修正高亮字段。",
    executorProviderLabel: "执行器 provider",
    expandAction: "展开",
    editableConfigurationLinearClaimAssigneeFieldRequired:
      "保存此托管 LINEAR worker 前请输入认领 assignee 字段。",
    editableConfigurationLinearMappingStateRequired:
      "保存此托管 LINEAR worker 前请输入映射 state。",
    editableConfigurationLinearMappingWorkTypeRequired:
      "保存此托管 LINEAR worker 前请输入映射 work type。",
    linearClaimAssigneeFieldFieldHelp:
      "可选。认领 issue 时使用的 LINEAR 字段路径（例如 assignee.email）。若此 worker 已配置 claim，则必填。",
    linearClaimAssigneeFieldLabel: "认领 assignee 字段",
    linearMappingStateFieldHelp:
      "从轮询的 LINEAR issue 创建的 submission 所分配的初始 work state。",
    linearMappingStateFieldLabel: "映射 state",
    linearMappingWorkTypeFieldHelp:
      "从轮询的 LINEAR issue 创建的 submission 所分配的 work type。",
    linearMappingWorkTypeFieldLabel: "映射 work type",
    linearPollIntervalFieldHelp:
      "可选。轮询器检查 LINEAR 新 issue 的频率（例如 30s 或 5m）。",
    linearPollIntervalFieldLabel: "轮询间隔",
    linearStateIdsFieldHelp:
      "可选。每行一个 LINEAR 工作流 state ID，用于过滤轮询范围。",
    linearStateIdsFieldLabel: "State ID",
    linearTeamIdsFieldHelp: "可选。每行一个 LINEAR team ID，用于限制轮询范围。",
    linearTeamIdsFieldLabel: "Team ID",
    modelFieldHelp: "可选。留空将使用 provider 默认模型标识符。",
    modelLabel: "模型",
    modelLocalityLabel: "模型位置",
    modelProviderFieldHelp:
      "模型 worker 必填。provider 决定路由和默认模型行为。",
    modelProviderLabel: "模型 provider",
    nameFieldLabel: "Worker 名称",
    notConfiguredOptionLabel: "未配置",
    notConfiguredValue: "未配置",
    providerFieldLabel: "托管 provider",
    referencingWorkstationsEmpty:
      "运行中的工厂定义中没有 workstation 引用此 worker。",
    referencingWorkstationsHeading: "引用 workstation",
    skipPermissionsFieldHelp:
      "启用后，支持的 model provider 可在执行期间跳过权限提示。",
    skipPermissionsFieldLabel: "跳过 provider 权限",
    stopTokenFieldHelp:
      "可选。Worker 拥有的标记；出现时可将面向模型的输出视为完成。与 workstation 停止词无关。",
    stopTokenFieldLabel: "停止标记",
    timeoutFieldHelp: "可选。限制 worker 运行的最长时间（例如 30s、5m、1h）。",
    timeoutFieldLabel: "执行超时",
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
  const catalog = resolveLocalizedMessages(
    workerDetailMessagesByLocale,
    locale,
  );

  return {
    ...catalog,
    ...enumMessages,
  };
}
