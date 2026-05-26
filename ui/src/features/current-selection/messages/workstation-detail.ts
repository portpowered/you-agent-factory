// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: feature-local locale catalogs keep required language coverage in one typed asset set.
import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { WorkstationDetailMessages } from "./workstation-detail-types";
import { getWorkstationDetailEnumMessages } from "./workstation-detail-enums";

type WorkstationDetailCatalogMessages = Omit<
  WorkstationDetailMessages,
  "localizeWorkstationBehavior" | "localizeWorkstationKind"
>;

const singularPlural = (count: number, singular: string, plural: string) =>
  `${count} ${count === 1 ? singular : plural}`;

const workstationDetailMessagesByLocale = {
  en: {
    activeRunsLabel: "Active runs",
    activeWorkEmpty: "No active work is running on this workstation.",
    activeWorkHeading: "Active work",
    collapseAction: "Collapse",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable worker and prompt values for the selected workstation.",
    editableConfigurationErrorPrefix: "Editable configuration unavailable.",
    editableConfigurationCollapseActionLabel: "Collapse editable configuration",
    editableConfigurationExpandActionLabel: "Expand editable configuration",
    editableConfigurationHeading: "Configuration",
    editableConfigurationDirtyStatus:
      "You have unsaved changes for this workstation.",
    editableConfigurationDraftNote:
      "Changes stay local to this edit session until you save the running factory.",
    editableConfigurationModelSharedWorkerHint:
      "Model edits are disabled here because this workstation shares its worker with other workstations.",
    editableConfigurationOverwriteWarning: (fields) =>
      `The running factory changed after you started editing. Saving now will overwrite newer server values for ${fields}.`,
    editableConfigurationOverwriteWarningDetail:
      "Review the latest runtime values before saving, or keep editing if this draft should replace them.",
    editableConfigurationSaveAction: "Save changes",
    editableConfigurationSaveBusyAction: "Saving...",
    editableConfigurationSaveConfirmationCancelAction: "Cancel",
    editableConfigurationSaveConfirmationConfirmAction: "Overwrite factory",
    editableConfigurationSaveConfirmationDescription:
      "Saving will overwrite the running factory definition with the kind, worker, and prompt values in this workstation draft.",
    editableConfigurationSaveConflictConfirmationDescription: (fields) =>
      `Saving will overwrite newer server values for ${fields} with the draft currently shown in the editor.`,
    editableConfigurationSaveConfirmationTitle:
      "Overwrite the running factory definition?",
    editableConfigurationSaveErrorPrefix: "Saving failed.",
    editableConfigurationSaveSuccess:
      "Running factory saved. The editable workstation values were refreshed to the saved definition.",
    editableConfigurationLoading:
      "Loading the current factory definition for this workstation.",
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this workstation.",
    editableConfigurationBehaviorPollerWorkerUnsupported:
      "Poller workstations must use a script or hosted worker before saving this workstation.",
    editableConfigurationPromptRequired:
      "Enter a prompt before saving this workstation.",
    editableConfigurationPromptEditorLoading:
      "Starting the prompt editor for this workstation.",
    editableConfigurationPromptEditorError:
      "The prompt editor could not be started. Reload this workstation and try again.",
    editableConfigurationPromptValidationLoading:
      "Validating prompt variables for the current draft.",
    editableConfigurationPromptValidationFallbackError:
      "Prompt validation could not be completed.",
    editableConfigurationPromptValidationErrorPrefix:
      "Prompt validation unavailable.",
    editableConfigurationPromptDiagnosticsSummary:
      "Resolve the highlighted prompt diagnostics before saving this workstation.",
    editableConfigurationPromptDiagnosticsHeading: "Prompt diagnostics",
    editableConfigurationPromptSyntaxDiagnosticLabel: "Template syntax",
    editableConfigurationPromptVariableDiagnosticLabel: "Variable access",
    editableConfigurationPromptValidationDetail:
      "Save stays disabled until the prompt validates cleanly for this workstation context.",
    editableConfigurationPromptHelpLoading:
      "Loading available prompt variables for this workstation.",
    editableConfigurationPromptHelpEmpty:
      "No prompt variable help is available for this workstation.",
    editableConfigurationPromptHelpFallbackError:
      "Prompt variable help could not be loaded.",
    editableConfigurationPromptHelpErrorPrefix:
      "Prompt variable help unavailable.",
    editableConfigurationPromptAutocompleteSummary: (variableCount, inputCount) =>
      `Autocomplete is ready with ${singularPlural(variableCount, "variable", "variables")} for ${singularPlural(inputCount, "authored input", "authored inputs")}.`,
    editableConfigurationPromptAutocompleteDetail:
      "Type inside {{ ... }} to see suggestions, or open Monaco completion manually anywhere in the prompt editor.",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
    editableConfigurationWorkerMissing:
      "The selected workstation references a worker that is no longer available in the current factory definition. Reload current selection and choose another worker.",
    editableConfigurationWorkerOptionsEmpty:
      "No current workers are available for this workstation. Add a worker to the factory before editing this field.",
    editableConfigurationWorkerRequired:
      "Select a worker before saving this workstation.",
    editableConfigurationWorkerUnavailable:
      "The selected worker is no longer available. Choose another worker before saving this workstation.",
    editableConfigurationWorkerUnavailablePrefix:
      "Worker selection unavailable.",
    modelFieldLabel: "Model",
    notConfiguredValue: "Not configured",
    promptFieldLabel: "Prompt",
    templateFieldLabel: "Template",
    workerFieldLabel: "Worker",
    currentDispatchLabel: "Current dispatch",
    dispatchLabel: "Dispatch",
    elapsedLabel: "Elapsed",
    expandAction: "Expand",
    historyRequestCountLabel: (count) =>
      singularPlural(count, "request", "requests"),
    historyRunCountLabel: (count) => singularPlural(count, "run", "runs"),
    historicalRequestsLabel: "Historical requests",
    historicalRunsLabel: "Historical runs",
    inputWorkTypesLabel: "Input work types",
    kindDefaultValue: "standard",
    kindLabel: "Kind",
    noWorkstationRequests:
      "No workstation requests have been recorded for this workstation yet.",
    noWorkstationRuns:
      "No workstation runs have been recorded for this workstation yet.",
    openRequestAction: "Open request",
    openRequestDetailsAction: "Open request details",
    openNamedWorkItemAction: (workItemLabel) => `Open ${workItemLabel}`,
    openWorkItemAction: "Open work item",
    outputWorkTypesLabel: "Output work types",
    projectedWorkstationRequestSummary: "Projected workstation request",
    providerSummary: (provider, model) =>
      `Provider ${provider}${model ? ` / ${model}` : ""}`,
    requestDetailsUnavailable: (dispatchId) =>
      `Request details unavailable for dispatch ${dispatchId}.`,
    requestHistoryHeading: "Request history",
    requestSelectedAction: "Request selected",
    requestStatusStartedAgo: (elapsed) => `Started ${elapsed} ago`,
    runnerCapabilityImageInputLabel: "Image input",
    runnerCapabilitySessionResumeLabel: "Session resume",
    runnerCapabilityStructuredOutputLabel: "Structured output",
    runnerCapabilitySupportHeading: "Runner capability support",
    runnerCapabilitySupportedLabel: "Supported",
    runnerCapabilityUnsupportedLabel: "Unsupported",
    runnerCapabilityWorkingDirectoryLabel: "Working directory",
    runnerCapabilityWorktreeLabel: "Worktree selection",
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `Effective runner: ${runnerName} (${sourceLabel}).`,
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `Inherit factory runner (${runnerName})`,
    runnerInheritanceFactoryMissingLabel: "Inherit default runner (Codex)",
    runnerInheritanceFactorySummaryLabel: "Factory runner",
    runnerInheritanceWorkstationSummaryLabel: "Workstation runner",
    runnerLoadingValue: "Loading runner...",
    runnerSelectionDefaultLabel: "Default",
    runnerSelectionFactoryLabel: "Factory",
    runnerSelectionLegacyProviderLabel: "Legacy provider",
    runnerSelectionWorkstationLabel: "Workstation",
    runHistoryHeading: "Run history",
    providerSessionLogAction: "Codex session log",
    providerSessionLogUnavailable: "Session log unavailable",
    providerSessionSelectedAction: "Session selected",
    providerSessionSelectAction: "Inspect session details",
    providerSessionSelectionUnavailable: "Session details unavailable",
    scriptCommandSummary: (command) => `Script command ${command}`,
    selectProviderSessionLabel: (sessionLabel, dispatchId) =>
      `Select provider session ${sessionLabel} for dispatch ${dispatchId}`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `Select request ${requestLabel} (${dispatchId})`,
    selectWorkItemLabel: (workItemLabel) => `Select work item ${workItemLabel}`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `Select workstation request ${dispatchId}`,
    selectedRequestLabel: (dispatchId) => `Selected request: ${dispatchId}.`,
    stationLabel: "Station",
    summaryHeading: "Workstation summary",
    selectedRunnerLabel: "Selected runner",
    traceIdLabel: "Trace ID",
    unknownActiveWorkLabel: "Unknown active work",
    unavailableValue: "Unavailable",
    unavailableRunnerValue: "Runner unavailable",
    unknownWorkerTypeValue: "Unknown",
    unknownWorkLabel: "Unknown work",
    workDetailsUnavailable: (dispatchId) =>
      `Work details unavailable for dispatch ${dispatchId}.`,
    workIdLabel: "Work ID",
    workSelectedAction: "Work selected",
    workerTypeLabel: "Worker type",
  },
  ja: {
    activeRunsLabel: "実行中のラン",
    activeWorkEmpty:
      "このワークステーションでは現在アクティブな作業は実行されていません。",
    activeWorkHeading: "アクティブな作業",
    collapseAction: "折りたたむ",
    editableConfigurationEmpty:
      "この選択中ワークステーションでは、実行中ファクトリー定義から編集可能な worker と prompt の値を取得できません。",
    editableConfigurationErrorPrefix: "編集可能な構成は利用できません。",
    editableConfigurationCollapseActionLabel: "編集可能な構成を折りたたむ",
    editableConfigurationExpandActionLabel: "編集可能な構成を展開",
    editableConfigurationHeading: "構成",
    editableConfigurationDirtyStatus:
      "このワークステーションには未保存の変更があります。",
    editableConfigurationDraftNote:
      "変更は、実行中ファクトリーを保存するまでこの編集セッション内だけに保持されます。",
    editableConfigurationModelSharedWorkerHint:
      "このワークステーションは他のワークステーションと同じワーカーを共有しているため、ここではモデルを編集できません。",
    editableConfigurationOverwriteWarning: (fields) =>
      `編集開始後に実行中ファクトリーが変更されました。今保存すると、${fields} の新しいサーバー値を上書きします。`,
    editableConfigurationOverwriteWarningDetail:
      "保存前に最新の実行時の値を確認するか、この下書きで置き換える場合はそのまま編集を続けてください。",
    editableConfigurationSaveAction: "変更を保存",
    editableConfigurationSaveBusyAction: "保存中...",
    editableConfigurationSaveConfirmationCancelAction: "キャンセル",
    editableConfigurationSaveConfirmationConfirmAction: "ファクトリーを上書き",
    editableConfigurationSaveConfirmationDescription:
      "保存すると、このワークステーション下書きの種別、worker、prompt の値で実行中ファクトリー定義を上書きします。",
    editableConfigurationSaveConflictConfirmationDescription: (fields) =>
      `保存すると、エディターに表示中の下書きで ${fields} の新しいサーバー値を上書きします。`,
    editableConfigurationSaveConfirmationTitle:
      "実行中ファクトリー定義を上書きしますか？",
    editableConfigurationSaveErrorPrefix: "保存に失敗しました。",
    editableConfigurationSaveSuccess:
      "実行中ファクトリーを保存しました。編集可能なワークステーション値は保存済み定義へ更新されました。",
    editableConfigurationLoading:
      "このワークステーション向けに現在のファクトリー定義を読み込んでいます。",
    editableConfigurationValidationStatus:
      "このワークステーションを保存する前に、強調表示された項目を修正してください。",
    editableConfigurationBehaviorPollerWorkerUnsupported:
      "このワークステーションを保存する前に、ポーラーのワークステーションではスクリプトまたは hosted worker を使用してください。",
    editableConfigurationPromptRequired:
      "このワークステーションを保存する前にプロンプトを入力してください。",
    editableConfigurationPromptEditorLoading:
      "このワークステーションのプロンプトエディターを起動しています。",
    editableConfigurationPromptEditorError:
      "プロンプトエディターを起動できませんでした。このワークステーションを再読み込みしてもう一度お試しください。",
    editableConfigurationPromptValidationLoading:
      "この下書きのプロンプト変数を検証しています。",
    editableConfigurationPromptValidationFallbackError:
      "プロンプト検証を完了できませんでした。",
    editableConfigurationPromptValidationErrorPrefix:
      "プロンプト検証は利用できません。",
    editableConfigurationPromptDiagnosticsSummary:
      "このワークステーションを保存する前に、強調表示されたプロンプト診断を修正してください。",
    editableConfigurationPromptDiagnosticsHeading: "プロンプト診断",
    editableConfigurationPromptSyntaxDiagnosticLabel: "テンプレート構文",
    editableConfigurationPromptVariableDiagnosticLabel: "変数アクセス",
    editableConfigurationPromptValidationDetail:
      "このワークステーション文脈でプロンプトの検証が成功するまで保存は無効のままです。",
    editableConfigurationPromptHelpLoading:
      "このワークステーションで利用可能なプロンプト変数を読み込んでいます。",
    editableConfigurationPromptHelpEmpty:
      "このワークステーションで利用できるプロンプト変数ヘルプはありません。",
    editableConfigurationPromptHelpFallbackError:
      "プロンプト変数ヘルプを読み込めませんでした。",
    editableConfigurationPromptHelpErrorPrefix:
      "プロンプト変数ヘルプは利用できません。",
    editableConfigurationPromptAutocompleteSummary: (variableCount, inputCount) =>
      `${inputCount} 件の入力コンテキストで ${variableCount} 件の補完候補を利用できます。`,
    editableConfigurationPromptAutocompleteDetail:
      "{{ ... }} の中で入力すると候補が表示され、エディターの補完コマンドで手動表示することもできます。",
    editableConfigurationSaveFallbackError:
      "実行中ファクトリーを保存できませんでした。",
    editableConfigurationWorkerMissing:
      "選択中のワークステーションは、現在のファクトリー定義に存在しないワーカーを参照しています。現在の選択を再読み込みして別のワーカーを選択してください。",
    editableConfigurationWorkerOptionsEmpty:
      "このワークステーションで利用できる現在のワーカーがありません。このフィールドを編集する前にファクトリーへワーカーを追加してください。",
    editableConfigurationWorkerRequired:
      "このワークステーションを保存する前にワーカーを選択してください。",
    editableConfigurationWorkerUnavailable:
      "選択したワーカーは利用できなくなりました。保存前に別のワーカーを選択してください。",
    editableConfigurationWorkerUnavailablePrefix:
      "ワーカー選択は利用できません。",
    modelFieldLabel: "モデル",
    notConfiguredValue: "未設定",
    promptFieldLabel: "プロンプト",
    templateFieldLabel: "テンプレート",
    workerFieldLabel: "ワーカー",
    currentDispatchLabel: "現在のディスパッチ",
    dispatchLabel: "ディスパッチ",
    elapsedLabel: "経過時間",
    expandAction: "展開",
    historyRequestCountLabel: (count) => `${count} 件のリクエスト`,
    historyRunCountLabel: (count) => `${count} 件のラン`,
    historicalRequestsLabel: "過去のリクエスト",
    historicalRunsLabel: "過去のラン",
    inputWorkTypesLabel: "入力ワークタイプ",
    kindDefaultValue: "standard",
    kindLabel: "種別",
    noWorkstationRequests:
      "このワークステーションではまだワークステーションリクエストが記録されていません。",
    noWorkstationRuns:
      "このワークステーションではまだワークステーションのランが記録されていません。",
    openRequestAction: "リクエストを開く",
    openRequestDetailsAction: "リクエスト詳細を開く",
    openNamedWorkItemAction: (workItemLabel) => `${workItemLabel} を開く`,
    openWorkItemAction: "ワークアイテムを開く",
    outputWorkTypesLabel: "出力ワークタイプ",
    projectedWorkstationRequestSummary:
      "投影されたワークステーションリクエスト",
    providerSummary: (provider, model) =>
      `プロバイダー ${provider}${model ? ` / ${model}` : ""}`,
    requestDetailsUnavailable: (dispatchId) =>
      `ディスパッチ ${dispatchId} のリクエスト詳細は利用できません。`,
    requestHistoryHeading: "リクエスト履歴",
    requestSelectedAction: "リクエストを選択済み",
    requestStatusStartedAgo: (elapsed) => `${elapsed} 前に開始`,
    runnerCapabilityImageInputLabel: "画像入力",
    runnerCapabilitySessionResumeLabel: "セッション再開",
    runnerCapabilityStructuredOutputLabel: "構造化出力",
    runnerCapabilitySupportHeading: "Runner の機能サポート",
    runnerCapabilitySupportedLabel: "対応",
    runnerCapabilityUnsupportedLabel: "未対応",
    runnerCapabilityWorkingDirectoryLabel: "作業ディレクトリ",
    runnerCapabilityWorktreeLabel: "worktree 選択",
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `有効な runner: ${runnerName}（${sourceLabel}）。`,
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `ファクトリー runner を継承 (${runnerName})`,
    runnerInheritanceFactoryMissingLabel: "既定 runner (Codex) を継承",
    runnerInheritanceFactorySummaryLabel: "ファクトリー runner",
    runnerInheritanceWorkstationSummaryLabel: "ワークステーション runner",
    runnerLoadingValue: "runner を読み込み中...",
    runnerSelectionDefaultLabel: "既定",
    runnerSelectionFactoryLabel: "ファクトリー",
    runnerSelectionLegacyProviderLabel: "旧 provider",
    runnerSelectionWorkstationLabel: "ワークステーション",
    runHistoryHeading: "ラン履歴",
    providerSessionLogAction: "Codex セッションログ",
    providerSessionLogUnavailable: "セッションログは利用できません",
    providerSessionSelectedAction: "セッションを選択済み",
    providerSessionSelectAction: "セッション詳細を表示",
    providerSessionSelectionUnavailable: "セッション詳細は利用できません",
    scriptCommandSummary: (command) => `スクリプトコマンド ${command}`,
    selectProviderSessionLabel: (sessionLabel, dispatchId) =>
      `ディスパッチ ${dispatchId} の provider session ${sessionLabel} を選択`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `リクエスト ${requestLabel} (${dispatchId}) を選択`,
    selectWorkItemLabel: (workItemLabel) =>
      `ワークアイテム ${workItemLabel} を選択`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `ワークステーションリクエスト ${dispatchId} を選択`,
    selectedRequestLabel: (dispatchId) => `選択中のリクエスト: ${dispatchId}。`,
    stationLabel: "ステーション",
    summaryHeading: "ワークステーション概要",
    selectedRunnerLabel: "選択中の runner",
    traceIdLabel: "トレース ID",
    unknownActiveWorkLabel: "不明なアクティブ作業",
    unavailableValue: "利用不可",
    unavailableRunnerValue: "runner は利用できません",
    unknownWorkerTypeValue: "不明",
    unknownWorkLabel: "不明な作業",
    workDetailsUnavailable: (dispatchId) =>
      `ディスパッチ ${dispatchId} の作業詳細は利用できません。`,
    workIdLabel: "ワーク ID",
    workSelectedAction: "ワークを選択済み",
    workerTypeLabel: "ワーカータイプ",
  },
  ko: {
    activeRunsLabel: "활성 실행",
    activeWorkEmpty: "이 워크스테이션에서 현재 실행 중인 활성 작업이 없습니다.",
    activeWorkHeading: "활성 작업",
    collapseAction: "접기",
    editableConfigurationEmpty:
      "선택한 워크스테이션에 대해 실행 중인 팩토리 정의에서 편집 가능한 worker 및 prompt 값을 찾을 수 없습니다.",
    editableConfigurationErrorPrefix: "편집 가능한 구성을 사용할 수 없습니다.",
    editableConfigurationCollapseActionLabel: "편집 가능한 구성 접기",
    editableConfigurationExpandActionLabel: "편집 가능한 구성 펼치기",
    editableConfigurationHeading: "구성",
    editableConfigurationDirtyStatus:
      "이 워크스테이션에 저장되지 않은 변경 사항이 있습니다.",
    editableConfigurationDraftNote:
      "변경 사항은 실행 중인 팩토리를 저장할 때까지 이 편집 세션에만 로컬로 유지됩니다.",
    editableConfigurationModelSharedWorkerHint:
      "이 워크스테이션은 다른 워크스테이션과 같은 워커를 공유하므로 여기서는 모델을 편집할 수 없습니다.",
    editableConfigurationOverwriteWarning: (fields) =>
      `편집을 시작한 뒤 실행 중인 팩토리가 변경되었습니다. 지금 저장하면 ${fields}의 최신 서버 값을 덮어쓰게 됩니다.`,
    editableConfigurationOverwriteWarningDetail:
      "저장하기 전에 최신 런타임 값을 검토하거나, 이 초안으로 대체하려면 계속 편집하세요.",
    editableConfigurationSaveAction: "변경 사항 저장",
    editableConfigurationSaveBusyAction: "저장 중...",
    editableConfigurationSaveConfirmationCancelAction: "취소",
    editableConfigurationSaveConfirmationConfirmAction: "팩토리 덮어쓰기",
    editableConfigurationSaveConfirmationDescription:
      "저장하면 이 워크스테이션 초안의 종류, worker, prompt 값으로 실행 중인 팩토리 정의를 덮어씁니다.",
    editableConfigurationSaveConflictConfirmationDescription: (fields) =>
      `저장하면 편집기에 표시된 초안으로 ${fields}의 최신 서버 값을 덮어씁니다.`,
    editableConfigurationSaveConfirmationTitle:
      "실행 중인 팩토리 정의를 덮어쓸까요?",
    editableConfigurationSaveErrorPrefix: "저장에 실패했습니다.",
    editableConfigurationSaveSuccess:
      "실행 중인 팩토리를 저장했습니다. 편집 가능한 워크스테이션 값이 저장된 정의로 새로 고쳐졌습니다.",
    editableConfigurationLoading:
      "이 워크스테이션의 현재 팩토리 정의를 불러오는 중입니다.",
    editableConfigurationValidationStatus:
      "이 워크스테이션을 저장하기 전에 강조 표시된 필드를 수정하세요.",
    editableConfigurationBehaviorPollerWorkerUnsupported:
      "이 워크스테이션을 저장하기 전에 폴러 워크스테이션에는 스크립트 또는 hosted worker를 사용하세요.",
    editableConfigurationPromptRequired:
      "이 워크스테이션을 저장하기 전에 프롬프트를 입력하세요.",
    editableConfigurationPromptEditorLoading:
      "이 워크스테이션의 프롬프트 편집기를 시작하는 중입니다.",
    editableConfigurationPromptEditorError:
      "프롬프트 편집기를 시작할 수 없습니다. 이 워크스테이션을 다시 불러오고 다시 시도하세요.",
    editableConfigurationPromptValidationLoading:
      "현재 초안의 프롬프트 변수를 검증하는 중입니다.",
    editableConfigurationPromptValidationFallbackError:
      "프롬프트 검증을 완료할 수 없습니다.",
    editableConfigurationPromptValidationErrorPrefix:
      "프롬프트 검증을 사용할 수 없습니다.",
    editableConfigurationPromptDiagnosticsSummary:
      "이 워크스테이션을 저장하기 전에 강조 표시된 프롬프트 진단을 해결하세요.",
    editableConfigurationPromptDiagnosticsHeading: "프롬프트 진단",
    editableConfigurationPromptSyntaxDiagnosticLabel: "템플릿 구문",
    editableConfigurationPromptVariableDiagnosticLabel: "변수 접근",
    editableConfigurationPromptValidationDetail:
      "이 워크스테이션 문맥에서 프롬프트가 정상 검증될 때까지 저장은 비활성화됩니다.",
    editableConfigurationPromptHelpLoading:
      "이 워크스테이션에서 사용할 수 있는 프롬프트 변수를 불러오는 중입니다.",
    editableConfigurationPromptHelpEmpty:
      "이 워크스테이션에는 사용할 수 있는 프롬프트 변수 도움말이 없습니다.",
    editableConfigurationPromptHelpFallbackError:
      "프롬프트 변수 도움말을 불러올 수 없습니다.",
    editableConfigurationPromptHelpErrorPrefix:
      "프롬프트 변수 도움말을 사용할 수 없습니다.",
    editableConfigurationPromptAutocompleteSummary: (variableCount, inputCount) =>
      `${inputCount}개의 입력 컨텍스트에서 ${variableCount}개의 자동완성 변수를 사용할 수 있습니다.`,
    editableConfigurationPromptAutocompleteDetail:
      "{{ ... }} 안에서 입력하면 추천이 나타나고, 프롬프트 편집기 어디서나 Monaco 자동완성 명령으로 수동 호출할 수도 있습니다.",
    editableConfigurationSaveFallbackError:
      "실행 중인 팩토리를 저장할 수 없습니다.",
    editableConfigurationWorkerMissing:
      "선택한 워크스테이션이 현재 팩토리 정의에 더 이상 없는 워커를 참조합니다. 현재 선택을 다시 불러오고 다른 워커를 선택하세요.",
    editableConfigurationWorkerOptionsEmpty:
      "이 워크스테이션에서 사용할 수 있는 현재 워커가 없습니다. 이 필드를 편집하기 전에 팩토리에 워커를 추가하세요.",
    editableConfigurationWorkerRequired:
      "이 워크스테이션을 저장하기 전에 워커를 선택하세요.",
    editableConfigurationWorkerUnavailable:
      "선택한 워커를 더 이상 사용할 수 없습니다. 저장하기 전에 다른 워커를 선택하세요.",
    editableConfigurationWorkerUnavailablePrefix:
      "워커 선택을 사용할 수 없습니다.",
    modelFieldLabel: "모델",
    notConfiguredValue: "구성되지 않음",
    promptFieldLabel: "프롬프트",
    templateFieldLabel: "템플릿",
    workerFieldLabel: "워커",
    currentDispatchLabel: "현재 디스패치",
    dispatchLabel: "디스패치",
    elapsedLabel: "경과 시간",
    expandAction: "펼치기",
    historyRequestCountLabel: (count) => `${count}개 요청`,
    historyRunCountLabel: (count) => `${count}개 실행`,
    historicalRequestsLabel: "이전 요청",
    historicalRunsLabel: "이전 실행",
    inputWorkTypesLabel: "입력 작업 유형",
    kindDefaultValue: "standard",
    kindLabel: "종류",
    noWorkstationRequests:
      "이 워크스테이션에는 아직 워크스테이션 요청 기록이 없습니다.",
    noWorkstationRuns:
      "이 워크스테이션에는 아직 워크스테이션 실행 기록이 없습니다.",
    openRequestAction: "요청 열기",
    openRequestDetailsAction: "요청 세부정보 열기",
    openNamedWorkItemAction: (workItemLabel) => `${workItemLabel} 열기`,
    openWorkItemAction: "작업 항목 열기",
    outputWorkTypesLabel: "출력 작업 유형",
    projectedWorkstationRequestSummary: "예상 워크스테이션 요청",
    providerSummary: (provider, model) =>
      `공급자 ${provider}${model ? ` / ${model}` : ""}`,
    requestDetailsUnavailable: (dispatchId) =>
      `디스패치 ${dispatchId}의 요청 세부정보를 사용할 수 없습니다.`,
    requestHistoryHeading: "요청 기록",
    requestSelectedAction: "요청 선택됨",
    requestStatusStartedAgo: (elapsed) => `${elapsed} 전에 시작됨`,
    runnerCapabilityImageInputLabel: "이미지 입력",
    runnerCapabilitySessionResumeLabel: "세션 재개",
    runnerCapabilityStructuredOutputLabel: "구조화된 출력",
    runnerCapabilitySupportHeading: "Runner 기능 지원",
    runnerCapabilitySupportedLabel: "지원",
    runnerCapabilityUnsupportedLabel: "미지원",
    runnerCapabilityWorkingDirectoryLabel: "작업 디렉터리",
    runnerCapabilityWorktreeLabel: "worktree 선택",
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `유효 runner: ${runnerName} (${sourceLabel}).`,
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `팩토리 runner 상속 (${runnerName})`,
    runnerInheritanceFactoryMissingLabel: "기본 runner (Codex) 상속",
    runnerInheritanceFactorySummaryLabel: "팩토리 runner",
    runnerInheritanceWorkstationSummaryLabel: "워크스테이션 runner",
    runnerLoadingValue: "runner 불러오는 중...",
    runnerSelectionDefaultLabel: "기본값",
    runnerSelectionFactoryLabel: "팩토리",
    runnerSelectionLegacyProviderLabel: "레거시 provider",
    runnerSelectionWorkstationLabel: "워크스테이션",
    runHistoryHeading: "실행 기록",
    providerSessionLogAction: "Codex 세션 로그",
    providerSessionLogUnavailable: "세션 로그를 사용할 수 없음",
    providerSessionSelectedAction: "세션 선택됨",
    providerSessionSelectAction: "세션 세부정보 보기",
    providerSessionSelectionUnavailable: "세션 세부정보를 사용할 수 없음",
    scriptCommandSummary: (command) => `스크립트 명령 ${command}`,
    selectProviderSessionLabel: (sessionLabel, dispatchId) =>
      `디스패치 ${dispatchId}의 provider session ${sessionLabel} 선택`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `요청 ${requestLabel} (${dispatchId}) 선택`,
    selectWorkItemLabel: (workItemLabel) => `작업 항목 ${workItemLabel} 선택`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `워크스테이션 요청 ${dispatchId} 선택`,
    selectedRequestLabel: (dispatchId) => `선택된 요청: ${dispatchId}.`,
    stationLabel: "스테이션",
    summaryHeading: "워크스테이션 요약",
    selectedRunnerLabel: "선택된 runner",
    traceIdLabel: "추적 ID",
    unknownActiveWorkLabel: "알 수 없는 활성 작업",
    unavailableValue: "사용할 수 없음",
    unavailableRunnerValue: "runner 를 사용할 수 없음",
    unknownWorkerTypeValue: "알 수 없음",
    unknownWorkLabel: "알 수 없는 작업",
    workDetailsUnavailable: (dispatchId) =>
      `디스패치 ${dispatchId}의 작업 세부정보를 사용할 수 없습니다.`,
    workIdLabel: "작업 ID",
    workSelectedAction: "작업 선택됨",
    workerTypeLabel: "워커 유형",
  },
  "zh-CN": {
    activeRunsLabel: "活动运行",
    activeWorkEmpty: "此工作站当前没有正在运行的活动工作。",
    activeWorkHeading: "活动工作",
    collapseAction: "收起",
    editableConfigurationEmpty:
      "运行中的工厂定义没有为所选工作站公开可编辑的 worker 和 prompt 值。",
    editableConfigurationErrorPrefix: "无法提供可编辑配置。",
    editableConfigurationCollapseActionLabel: "收起可编辑配置",
    editableConfigurationExpandActionLabel: "展开可编辑配置",
    editableConfigurationHeading: "配置",
    editableConfigurationDirtyStatus: "此工作站存在未保存的更改。",
    editableConfigurationDraftNote:
      "在保存运行中的工厂之前，更改只会保留在当前编辑会话中。",
    editableConfigurationModelSharedWorkerHint:
      "此工作站与其他工作站共享同一个 worker，因此这里不能编辑模型。",
    editableConfigurationOverwriteWarning: (fields) =>
      `你开始编辑后，运行中的工厂已发生变化。现在保存将覆盖 ${fields} 的较新服务器值。`,
    editableConfigurationOverwriteWarningDetail:
      "保存前请先检查最新运行时值；如果此草稿就应该替换它们，也可以继续编辑。",
    editableConfigurationSaveAction: "保存更改",
    editableConfigurationSaveBusyAction: "保存中...",
    editableConfigurationSaveConfirmationCancelAction: "取消",
    editableConfigurationSaveConfirmationConfirmAction: "覆盖工厂",
    editableConfigurationSaveConfirmationDescription:
      "保存将使用此工作站草稿中的类型、worker 和 prompt 值覆盖运行中的工厂定义。",
    editableConfigurationSaveConflictConfirmationDescription: (fields) =>
      `保存将使用编辑器中当前草稿覆盖 ${fields} 的较新服务器值。`,
    editableConfigurationSaveConfirmationTitle: "要覆盖运行中的工厂定义吗？",
    editableConfigurationSaveErrorPrefix: "保存失败。",
    editableConfigurationSaveSuccess:
      "运行中的工厂已保存。可编辑的工作站值已刷新为保存后的定义。",
    editableConfigurationLoading: "正在加载此工作站的当前工厂定义。",
    editableConfigurationValidationStatus: "请先修正高亮字段，再保存此工作站。",
    editableConfigurationBehaviorPollerWorkerUnsupported:
      "保存此工作站前，请先为轮询器工作站选择脚本或 hosted worker。",
    editableConfigurationPromptRequired: "保存此工作站前请输入提示词。",
    editableConfigurationPromptEditorLoading: "正在启动此工作站的提示词编辑器。",
    editableConfigurationPromptEditorError:
      "无法启动提示词编辑器。请重新加载此工作站后重试。",
    editableConfigurationPromptValidationLoading: "正在校验当前草稿中的提示词变量。",
    editableConfigurationPromptValidationFallbackError: "无法完成提示词校验。",
    editableConfigurationPromptValidationErrorPrefix: "提示词校验不可用。",
    editableConfigurationPromptDiagnosticsSummary:
      "保存此工作站前，请先解决高亮显示的提示词诊断问题。",
    editableConfigurationPromptDiagnosticsHeading: "提示词诊断",
    editableConfigurationPromptSyntaxDiagnosticLabel: "模板语法",
    editableConfigurationPromptVariableDiagnosticLabel: "变量访问",
    editableConfigurationPromptValidationDetail:
      "只有当此工作站上下文中的提示词通过校验后，保存才会重新可用。",
    editableConfigurationPromptHelpLoading:
      "正在加载此工作站可用的提示词变量。",
    editableConfigurationPromptHelpEmpty:
      "此工作站没有可用的提示词变量帮助信息。",
    editableConfigurationPromptHelpFallbackError:
      "无法加载提示词变量帮助信息。",
    editableConfigurationPromptHelpErrorPrefix: "提示词变量帮助信息不可用。",
    editableConfigurationPromptAutocompleteSummary: (variableCount, inputCount) =>
      `自动补全已就绪，可为 ${inputCount} 个已编写输入上下文提供 ${variableCount} 个变量候选。`,
    editableConfigurationPromptAutocompleteDetail:
      "在 {{ ... }} 内输入时会显示建议，也可以在提示词编辑器任意位置手动触发 Monaco 补全。",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
    editableConfigurationWorkerMissing:
      "所选工作站引用的工作器已不在当前工厂定义中。请重新加载当前选择并选择其他工作器。",
    editableConfigurationWorkerOptionsEmpty:
      "此工作站当前没有可用的工作器。请先向工厂添加工作器，再编辑此字段。",
    editableConfigurationWorkerRequired: "保存此工作站前请选择工作器。",
    editableConfigurationWorkerUnavailable:
      "所选工作器已不可用。保存前请选择其他工作器。",
    editableConfigurationWorkerUnavailablePrefix: "工作器选择不可用。",
    modelFieldLabel: "模型",
    notConfiguredValue: "未配置",
    promptFieldLabel: "提示词",
    templateFieldLabel: "模板",
    workerFieldLabel: "工作器",
    currentDispatchLabel: "当前分派",
    dispatchLabel: "分派",
    elapsedLabel: "已用时间",
    expandAction: "展开",
    historyRequestCountLabel: (count) => `${count} 个请求`,
    historyRunCountLabel: (count) => `${count} 次运行`,
    historicalRequestsLabel: "历史请求",
    historicalRunsLabel: "历史运行",
    inputWorkTypesLabel: "输入工作类型",
    kindDefaultValue: "standard",
    kindLabel: "类型",
    noWorkstationRequests: "此工作站尚未记录任何工作站请求。",
    noWorkstationRuns: "此工作站尚未记录任何工作站运行。",
    openRequestAction: "打开请求",
    openRequestDetailsAction: "打开请求详情",
    openNamedWorkItemAction: (workItemLabel) => `打开 ${workItemLabel}`,
    openWorkItemAction: "打开工作项",
    outputWorkTypesLabel: "输出工作类型",
    projectedWorkstationRequestSummary: "预测的工作站请求",
    providerSummary: (provider, model) =>
      `提供方 ${provider}${model ? ` / ${model}` : ""}`,
    requestDetailsUnavailable: (dispatchId) =>
      `无法提供分派 ${dispatchId} 的请求详情。`,
    requestHistoryHeading: "请求历史",
    requestSelectedAction: "请求已选中",
    requestStatusStartedAgo: (elapsed) => `开始于 ${elapsed} 前`,
    runnerCapabilityImageInputLabel: "图片输入",
    runnerCapabilitySessionResumeLabel: "会话恢复",
    runnerCapabilityStructuredOutputLabel: "结构化输出",
    runnerCapabilitySupportHeading: "Runner 能力支持",
    runnerCapabilitySupportedLabel: "支持",
    runnerCapabilityUnsupportedLabel: "不支持",
    runnerCapabilityWorkingDirectoryLabel: "工作目录",
    runnerCapabilityWorktreeLabel: "worktree 选择",
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `当前生效的 runner：${runnerName}（${sourceLabel}）。`,
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `继承工厂 runner（${runnerName}）`,
    runnerInheritanceFactoryMissingLabel: "继承默认 runner（Codex）",
    runnerInheritanceFactorySummaryLabel: "工厂 runner",
    runnerInheritanceWorkstationSummaryLabel: "工作站 runner",
    runnerLoadingValue: "正在加载 runner...",
    runnerSelectionDefaultLabel: "默认值",
    runnerSelectionFactoryLabel: "工厂",
    runnerSelectionLegacyProviderLabel: "旧 provider",
    runnerSelectionWorkstationLabel: "工作站",
    runHistoryHeading: "运行历史",
    providerSessionLogAction: "Codex 会话日志",
    providerSessionLogUnavailable: "会话日志不可用",
    providerSessionSelectedAction: "会话已选中",
    providerSessionSelectAction: "查看会话详情",
    providerSessionSelectionUnavailable: "会话详情不可用",
    scriptCommandSummary: (command) => `脚本命令 ${command}`,
    selectProviderSessionLabel: (sessionLabel, dispatchId) =>
      `选择调度 ${dispatchId} 的 provider session ${sessionLabel}`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `选择请求 ${requestLabel} (${dispatchId})`,
    selectWorkItemLabel: (workItemLabel) => `选择工作项 ${workItemLabel}`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `选择工作站请求 ${dispatchId}`,
    selectedRequestLabel: (dispatchId) => `已选择请求：${dispatchId}。`,
    stationLabel: "站点",
    summaryHeading: "工作站摘要",
    selectedRunnerLabel: "当前 runner",
    traceIdLabel: "跟踪 ID",
    unknownActiveWorkLabel: "未知活动工作",
    unavailableValue: "不可用",
    unavailableRunnerValue: "runner 不可用",
    unknownWorkerTypeValue: "未知",
    unknownWorkLabel: "未知工作",
    workDetailsUnavailable: (dispatchId) =>
      `无法提供分派 ${dispatchId} 的工作详情。`,
    workIdLabel: "工作 ID",
    workSelectedAction: "工作已选中",
    workerTypeLabel: "工作器类型",
  },
} satisfies LocalizedMessages<WorkstationDetailCatalogMessages>;

export function getWorkstationDetailMessages(
  locale?: string | null,
): WorkstationDetailMessages {
  const messages = resolveLocalizedMessages(
    workstationDetailMessagesByLocale,
    locale,
  );
  const enumMessages = getWorkstationDetailEnumMessages(locale);

  return {
    ...messages,
    localizeWorkstationBehavior: enumMessages.localizeWorkstationBehavior,
    localizeWorkstationKind: enumMessages.localizeWorkstationKind,
  };
}

export type { WorkstationDetailMessages } from "./workstation-detail-types";
export { workstationDetailMessagesByLocale };
