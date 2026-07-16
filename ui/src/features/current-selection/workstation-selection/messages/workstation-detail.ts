// biome-ignore-all lint/style/noExcessiveLinesPerFile: feature-local locale catalogs keep required language coverage in one typed asset set.
import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import { getWorkstationDetailEnumMessages } from "./workstation-detail-enums";
import type { WorkstationDetailMessages } from "./workstation-detail-types";

type WorkstationDetailCatalogMessages = Omit<
  WorkstationDetailMessages,
  | "localizeProviderSessionKind"
  | "localizeRunnerSelectionSource"
  | "localizeWorkstationBehavior"
  | "localizeWorkstationGuardType"
  | "localizeInputGuardType"
  | "localizeWorkstationKind"
  | "localizeWorkstationType"
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
    editableConfigurationModelSharedWorkerHint:
      "Model edits are disabled for shared workers.",
    editableConfigurationNameDuplicate: (workstationName) =>
      `A workstation named "${workstationName}" already exists in the running factory definition.`,
    editableConfigurationNameRequired:
      "Enter a workstation name before saving this workstation.",
    editableConfigurationResetAction: "Reset to latest",
    editableConfigurationServerFieldChangedHint:
      "The running factory changed this field while you were editing. Reset to latest to discard the local draft value.",
    editableConfigurationOverwriteWarning: (fields) =>
      `The running factory changed after you started editing. Saving now will overwrite newer server values for ${fields}.`,
    editableConfigurationOverwriteWarningDetail:
      "Confirm latest values or keep editing.",
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
    editableConfigurationSaveStaleVersionDetail:
      "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
    editableConfigurationSaveSuccess: (workstationName) =>
      `Running factory saved. ${workstationName} was updated in the running factory definition.`,
    editableConfigurationLoading:
      "Loading the current factory definition for this workstation.",
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this workstation.",
    editableConfigurationBehaviorPollerHint:
      "Poller workstations supervise a long-lived ingress worker. Configure the hosted or script worker separately; this workstation only binds the poller behavior and routes emitted work.",
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
      "Fix highlighted issues before saving.",
    editableConfigurationPromptFieldHint: "See prompt diagnostics below.",
    editableConfigurationPromptDiagnosticsHeading: "Prompt diagnostics",
    editableConfigurationPromptSyntaxDiagnosticLabel: "Template syntax",
    editableConfigurationPromptVariableDiagnosticLabel: "Variable access",
    editableConfigurationPromptHelpLoading:
      "Loading available prompt variables for this workstation.",
    editableConfigurationPromptHelpEmpty:
      "No prompt variable help is available for this workstation.",
    editableConfigurationPromptHelpFallbackError:
      "Prompt variable help could not be loaded.",
    editableConfigurationPromptHelpErrorPrefix:
      "Prompt variable help unavailable.",
    editableConfigurationPromptAutocompleteSummary: (
      variableCount,
      inputCount,
    ) =>
      `Autocomplete is ready with ${singularPlural(variableCount, "variable", "variables")} for ${singularPlural(inputCount, "authored input", "authored inputs")}.`,
    editableConfigurationPromptAutocompleteDetail:
      "Type inside {{ ... }} for suggestions.",
    editableConfigurationPromptHelpExpandActionLabel:
      "Open prompt variable help",
    editableConfigurationPromptHelpCollapseActionLabel:
      "Close prompt variable help",
    editableConfigurationPromptAvailableVariablesHeading: "Available variables",
    editableConfigurationPromptUnavailableAccessHeading: "Unavailable access",
    editableConfigurationPromptResizeHandleLabel: "Resize prompt editor height",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
    editableConfigurationWorkerMissing:
      "The selected workstation references a worker that is no longer available in the current factory definition. Reload current selection and choose another worker.",
    editableConfigurationWorkerOptionsEmpty:
      "No current workers are available for this workstation. Add a worker to the factory before editing this field.",
    editableConfigurationWorkerRequired:
      "Select a worker before saving this workstation.",
    editableConfigurationSharedWorkerScopeHint: (
      workerName,
      workstationNames,
    ) => `Worker ${workerName} is also used by ${workstationNames}.`,
    editableConfigurationWorkerUnavailable:
      "The selected worker is no longer available. Choose another worker before saving this workstation.",
    editableConfigurationWorkerUnavailablePrefix:
      "Worker selection unavailable.",
    editableConfigurationModelInvokeBindingDuplicate: (slotName) =>
      `Operation binding for slot "${slotName}" is declared more than once.`,
    editableConfigurationModelInvokeBindingRequired: (slotName) =>
      `Required slot "${slotName}" needs a selector, config content, or default content.`,
    editableConfigurationModelInvokeBindingsSummary:
      "Resolve the highlighted operation binding fields before saving this workstation.",
    editableConfigurationModelInvokeOperationInvalid:
      "Operation names must be uppercase letters, digits, or underscores.",
    editableConfigurationModelInvokeOperationMissing:
      "The selected operation is not declared on the chosen model worker.",
    editableConfigurationModelInvokeOperationOptionsEmpty:
      "The selected model worker does not declare any compatible operations.",
    editableConfigurationModelInvokeOperationRequired:
      "Select an operation before saving this workstation.",
    editableConfigurationModelInvokeWorkerOptionsEmpty:
      "No model workers with compatible operations are available in the current factory definition.",
    editableConfigurationModelInvokeWorkerRequired:
      "Select a model worker before choosing an operation.",
    modelInvokeBindingConfigContentFieldLabel: "Config content",
    modelInvokeBindingDefaultContentFieldLabel: "Default content",
    modelInvokeBindingsEmpty:
      "Select a worker operation to edit input slot bindings.",
    modelInvokeBindingsFieldHint:
      "Bindings resolve runtime input using selector fields, static config content, or default content. Leave optional slots empty to omit them.",
    modelInvokeBindingsFieldLabel: "Operation bindings",
    modelInvokeBindingOptionalSlotLabel: "optional",
    modelInvokeBindingRequiredSlotLabel: "required",
    modelInvokeBindingSelectorLabelFieldLabel: "Selector label",
    modelInvokeBindingSelectorRoleFieldLabel: "Selector role",
    modelInvokeBindingSelectorSlotFieldLabel: "Selector slot",
    modelInvokeBindingSelectorTypeFieldLabel: "Selector type",
    modelInvokeBindingSelectorTypeNoneOption: "Any type",
    modelInvokeBindingSlotHeading: (slotName, requirement) =>
      `${slotName} (${requirement})`,
    modelInvokeOperationFieldLabel: "Operation",
    editableConfigurationWorkstationOptionsEmpty:
      "No workstations are available in the current factory definition.",
    editableConfigurationWorkstationUnavailablePrefix:
      "Workstation selection unavailable.",
    editableConfigurationVisitCountMaxVisitsInvalid:
      "Max visits must be a positive whole number.",
    editableConfigurationVisitCountWorkstationInvalid: (workstation) =>
      `Counted workstation ${workstation} is not available in this factory.`,
    editableConfigurationVisitCountWorkstationRequired:
      "Select the workstation whose visits are counted.",
    editableConfigurationMatchesFieldsInputKeyRequired:
      "Enter a field selector for this guard.",
    editableConfigurationInputGuardMultipleGuards:
      "Each input slot can have at most one guard.",
    editableConfigurationInputGuardMatchInputRequired:
      "Select a peer input for this guard.",
    editableConfigurationInputGuardMatchInputInvalid: (workType) =>
      `Peer input ${workType} is not available on this workstation.`,
    editableConfigurationInputGuardMatchInputSelfReference:
      "Peer input cannot reference the same input slot.",
    editableConfigurationInputGuardParentInputRequired:
      "Select a parent input for this guard.",
    editableConfigurationInputGuardParentInputInvalid: (workType) =>
      `Parent input ${workType} is not available on this workstation.`,
    editableConfigurationInputGuardParentInputSelfReference:
      "Parent input cannot reference the same input slot.",
    editableConfigurationInputGuardSpawnedByInvalid: (workstation) =>
      `Spawned-by workstation ${workstation} is not available in this factory.`,
    matchesFieldsGuardInputKeyFieldLabel: "Field selector",
    editableConfigurationGuardSelectorEditorLoading:
      "Starting the field selector editor.",
    editableConfigurationGuardSelectorEditorError:
      "The field selector editor could not be started. Reload this workstation and try again.",
    modelFieldLabel: "Model",
    notConfiguredValue: "Not configured",
    promptFieldLabel: "Prompt",
    templateFieldLabel: "Template",
    workerFieldLabel: "Worker",
    workstationNameFieldLabel: "Workstation name",
    workstationGuardsHeading: "Workstation guards",
    workstationGuardsEmpty: "No workstation guards are configured.",
    workstationGuardsAddLabel: "Add guard",
    workstationGuardsAddPlaceholder: "Choose a guard type",
    workstationGuardsRemoveAction: "Remove guard",
    visitCountGuardWorkstationFieldLabel: "Counted workstation",
    visitCountGuardMaxVisitsFieldLabel: "Max visits",
    inputGuardMatchInputFieldLabel: "Peer input",
    inputGuardParentInputFieldLabel: "Parent input",
    inputGuardSpawnedByFieldLabel: "Spawned by (optional)",
    workstationInputGuardsHeading: "Input guards",
    workstationInputGuardsEmpty: "This workstation has no authored inputs.",
    workstationInputGuardTypeFieldLabel: "Input guard",
    workstationInputGuardNoneOption: "None",
    workstationInputGuardPeersEmpty:
      "Add another input on this workstation to configure peer-based guards.",
    workstationInputSlotHeading: (workType, state) => `${workType} · ${state}`,
    currentDispatchLabel: "Current dispatch",
    dispatchLabel: "Dispatch",
    elapsedLabel: "Elapsed",
    totalRuntimeLabel: "Total runtime",
    expandAction: "Expand",
    historyRequestCountLabel: (count) =>
      singularPlural(count, "request", "requests"),
    historyRunCountLabel: (count) => singularPlural(count, "run", "runs"),
    historicalRequestsLabel: "Historical requests",
    historicalRunsLabel: "Historical runs",
    inputWorkTypesLabel: "Input work types",
    kindDefaultValue: "STANDARD",
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
    requestStatusStartedAgo: (elapsed) => `Started ${elapsed}`,
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `Effective runner: ${runnerName} (${sourceLabel}).`,
    editableConfigurationCronExpiryWindowInvalid: (value) =>
      `expiry_window must be a positive duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronJitterInvalid: (value) =>
      `jitter must be a non-negative duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronScheduleInvalid: (schedule, detail) =>
      `invalid cron schedule ${JSON.stringify(schedule)}: ${detail}`,
    editableConfigurationCronScheduleRequired:
      "cron workstation requires non-empty 'schedule'",
    cronExpiryWindowFieldHint:
      "Optional positive Go duration after due time before stale cron work expires (for example 30s, 5m, 1h).",
    cronExpiryWindowFieldLabel: "Cron expiry window",
    cronJitterFieldHint:
      "Optional non-negative Go duration for maximum deterministic schedule jitter (for example 0s, 30s, 5m).",
    cronJitterFieldLabel: "Cron jitter",
    cronScheduleFieldHint:
      "Required five-field cron expression (for example */5 * * * *).",
    cronScheduleFieldLabel: "Cron schedule",
    cronTriggerAtStartFieldLabel: "Cron trigger at start",
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `Inherit factory runner (${runnerName})`,
    runnerInheritanceFactoryMissingLabel: "Inherit default runner (Codex)",
    runnerLoadingValue: "Loading runner...",
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
    unavailableWorkstationKindValue: "Workstation kind unavailable",
    unavailableWorkstationTypeValue: "Workstation type unavailable",
    unknownWorkerTypeValue: "Unknown",
    unknownWorkLabel: "Unknown work",
    workDetailsUnavailable: (dispatchId) =>
      `Work details unavailable for dispatch ${dispatchId}.`,
    workIdLabel: "Work ID",
    workSelectedAction: "Work selected",
    workerTypeLabel: "Worker type",
    workstationKindLoadingValue: "Loading workstation kind...",
    workstationTypeLabel: "Workstation type",
    workstationTypeLoadingValue: "Loading workstation type...",
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
    editableConfigurationModelSharedWorkerHint:
      "共有ワーカーのためモデル編集は無効です。",
    editableConfigurationNameDuplicate: (workstationName) =>
      `ワークステーション名 "${workstationName}" は、実行中のファクトリー定義にすでに存在します。`,
    editableConfigurationNameRequired:
      "このワークステーションを保存する前にワークステーション名を入力してください。",
    editableConfigurationResetAction: "最新へ戻す",
    editableConfigurationServerFieldChangedHint:
      "編集中に実行中ファクトリーのこの項目が更新されました。最新へ戻すと、この下書きのローカル値を破棄します。",
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
    editableConfigurationSaveStaleVersionDetail:
      "最新の実行中ファクトリー値を再読み込みするか、この下書きを保持したままエディターの更新後に再試行してください。",
    editableConfigurationSaveSuccess: (workstationName) =>
      `実行中ファクトリーを保存しました。${workstationName} は実行中ファクトリー定義で更新されました。`,
    editableConfigurationLoading:
      "このワークステーション向けに現在のファクトリー定義を読み込んでいます。",
    editableConfigurationValidationStatus:
      "このワークステーションを保存する前に、強調表示された項目を修正してください。",
    editableConfigurationBehaviorPollerHint:
      "ポーラーワークステーションは長寿命のイングレスワーカーを監督します。ホストまたはスクリプトワーカーは別途設定し、このワークステーションはポーラー動作の割り当てと出力ルーティングのみを担います。",
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
      "保存前に強調表示された問題を修正してください。",
    editableConfigurationPromptFieldHint:
      "下のプロンプト診断を確認してください。",
    editableConfigurationPromptDiagnosticsHeading: "プロンプト診断",
    editableConfigurationPromptSyntaxDiagnosticLabel: "テンプレート構文",
    editableConfigurationPromptVariableDiagnosticLabel: "変数アクセス",
    editableConfigurationPromptHelpLoading:
      "このワークステーションで利用可能なプロンプト変数を読み込んでいます。",
    editableConfigurationPromptHelpEmpty:
      "このワークステーションで利用できるプロンプト変数ヘルプはありません。",
    editableConfigurationPromptHelpFallbackError:
      "プロンプト変数ヘルプを読み込めませんでした。",
    editableConfigurationPromptHelpErrorPrefix:
      "プロンプト変数ヘルプは利用できません。",
    editableConfigurationPromptAutocompleteSummary: (
      variableCount,
      inputCount,
    ) =>
      `${inputCount} 件の入力コンテキストで ${variableCount} 件の補完候補を利用できます。`,
    editableConfigurationPromptAutocompleteDetail:
      "候補は {{ ... }} の中で入力しているときだけ表示されます。",
    editableConfigurationPromptHelpExpandActionLabel:
      "プロンプト変数ヘルプを開く",
    editableConfigurationPromptHelpCollapseActionLabel:
      "プロンプト変数ヘルプを閉じる",
    editableConfigurationPromptAvailableVariablesHeading: "利用可能な変数",
    editableConfigurationPromptUnavailableAccessHeading: "利用できないアクセス",
    editableConfigurationPromptResizeHandleLabel:
      "プロンプトエディターの高さを変更",
    editableConfigurationSaveFallbackError:
      "実行中ファクトリーを保存できませんでした。",
    editableConfigurationWorkerMissing:
      "選択中のワークステーションは、現在のファクトリー定義に存在しないワーカーを参照しています。現在の選択を再読み込みして別のワーカーを選択してください。",
    editableConfigurationWorkerOptionsEmpty:
      "このワークステーションで利用できる現在のワーカーがありません。このフィールドを編集する前にファクトリーへワーカーを追加してください。",
    editableConfigurationWorkerRequired:
      "このワークステーションを保存する前にワーカーを選択してください。",
    editableConfigurationSharedWorkerScopeHint: (
      workerName,
      workstationNames,
    ) => `ワーカー ${workerName} は ${workstationNames} でも使用されています。`,
    editableConfigurationWorkerUnavailable:
      "選択したワーカーは利用できなくなりました。保存前に別のワーカーを選択してください。",
    editableConfigurationWorkerUnavailablePrefix:
      "ワーカー選択は利用できません。",
    editableConfigurationModelInvokeBindingDuplicate: (slotName) =>
      `スロット "${slotName}" の操作バインドが重複しています。`,
    editableConfigurationModelInvokeBindingRequired: (slotName) =>
      `必須スロット "${slotName}" にはセレクター、設定コンテンツ、またはデフォルトコンテンツが必要です。`,
    editableConfigurationModelInvokeBindingsSummary:
      "保存する前に強調表示された操作バインド項目を解決してください。",
    editableConfigurationModelInvokeOperationInvalid:
      "操作名は大文字の英字、数字、アンダースコアのみ使用できます。",
    editableConfigurationModelInvokeOperationMissing:
      "選択した操作は選択したモデルワーカーで宣言されていません。",
    editableConfigurationModelInvokeOperationOptionsEmpty:
      "選択したモデルワーカーには互換性のある操作が宣言されていません。",
    editableConfigurationModelInvokeOperationRequired:
      "このワークステーションを保存する前に操作を選択してください。",
    editableConfigurationModelInvokeWorkerOptionsEmpty:
      "現在のファクトリー定義で互換性のある操作を持つモデルワーカーがありません。",
    editableConfigurationModelInvokeWorkerRequired:
      "操作を選択する前にモデルワーカーを選択してください。",
    modelInvokeBindingConfigContentFieldLabel: "設定コンテンツ",
    modelInvokeBindingDefaultContentFieldLabel: "デフォルトコンテンツ",
    modelInvokeBindingsEmpty:
      "入力スロットのバインドを編集するにはワーカー操作を選択してください。",
    modelInvokeBindingsFieldHint:
      "バインドはセレクター項目、静的設定コンテンツ、またはデフォルトコンテンツでランタイム入力を解決します。任意スロットは空のままにすると省略されます。",
    modelInvokeBindingsFieldLabel: "操作バインド",
    modelInvokeBindingOptionalSlotLabel: "任意",
    modelInvokeBindingRequiredSlotLabel: "必須",
    modelInvokeBindingSelectorLabelFieldLabel: "セレクターラベル",
    modelInvokeBindingSelectorRoleFieldLabel: "セレクターロール",
    modelInvokeBindingSelectorSlotFieldLabel: "セレクタースロット",
    modelInvokeBindingSelectorTypeFieldLabel: "セレクタータイプ",
    modelInvokeBindingSelectorTypeNoneOption: "任意のタイプ",
    modelInvokeBindingSlotHeading: (slotName, requirement) =>
      `${slotName}（${requirement}）`,
    modelInvokeOperationFieldLabel: "操作",
    editableConfigurationWorkstationOptionsEmpty:
      "現在のファクトリー定義に利用可能なワークステーションがありません。",
    editableConfigurationWorkstationUnavailablePrefix:
      "ワークステーション選択は利用できません。",
    editableConfigurationVisitCountMaxVisitsInvalid:
      "最大訪問回数は正の整数である必要があります。",
    editableConfigurationVisitCountWorkstationInvalid: (workstation) =>
      `カウント対象ワークステーション ${workstation} はこのファクトリーで利用できません。`,
    editableConfigurationVisitCountWorkstationRequired:
      "訪問回数をカウントするワークステーションを選択してください。",
    editableConfigurationMatchesFieldsInputKeyRequired:
      "このガードのフィールドセレクターを入力してください。",
    editableConfigurationInputGuardMultipleGuards:
      "各入力スロットには最大 1 つのガードしか設定できません。",
    editableConfigurationInputGuardMatchInputRequired:
      "このガードのピア入力を選択してください。",
    editableConfigurationInputGuardMatchInputInvalid: (workType) =>
      `ピア入力 ${workType} はこのワークステーションで利用できません。`,
    editableConfigurationInputGuardMatchInputSelfReference:
      "ピア入力は同じ入力スロットを参照できません。",
    editableConfigurationInputGuardParentInputRequired:
      "このガードの親入力を選択してください。",
    editableConfigurationInputGuardParentInputInvalid: (workType) =>
      `親入力 ${workType} はこのワークステーションで利用できません。`,
    editableConfigurationInputGuardParentInputSelfReference:
      "親入力は同じ入力スロットを参照できません。",
    editableConfigurationInputGuardSpawnedByInvalid: (workstation) =>
      `spawned-by ワークステーション ${workstation} はこのファクトリーで利用できません。`,
    matchesFieldsGuardInputKeyFieldLabel: "フィールドセレクター",
    editableConfigurationGuardSelectorEditorLoading:
      "フィールドセレクターエディターを起動しています。",
    editableConfigurationGuardSelectorEditorError:
      "フィールドセレクターエディターを起動できませんでした。このワークステーションを再読み込みして、もう一度お試しください。",
    modelFieldLabel: "モデル",
    notConfiguredValue: "未設定",
    promptFieldLabel: "プロンプト",
    templateFieldLabel: "テンプレート",
    workerFieldLabel: "ワーカー",
    workstationNameFieldLabel: "ワークステーション名",
    workstationGuardsHeading: "ワークステーションガード",
    workstationGuardsEmpty: "ワークステーションガードは設定されていません。",
    workstationGuardsAddLabel: "ガードを追加",
    workstationGuardsAddPlaceholder: "ガード種別を選択",
    workstationGuardsRemoveAction: "ガードを削除",
    visitCountGuardWorkstationFieldLabel: "カウント対象ワークステーション",
    visitCountGuardMaxVisitsFieldLabel: "最大訪問回数",
    inputGuardMatchInputFieldLabel: "ピア入力",
    inputGuardParentInputFieldLabel: "親入力",
    inputGuardSpawnedByFieldLabel: "生成元（任意）",
    workstationInputGuardsHeading: "入力ガード",
    workstationInputGuardsEmpty:
      "このワークステーションには作成済み入力がありません。",
    workstationInputGuardTypeFieldLabel: "入力ガード",
    workstationInputGuardNoneOption: "なし",
    workstationInputGuardPeersEmpty:
      "ピアベースのガードを設定するには、同じワークステーションに別の入力を追加してください。",
    workstationInputSlotHeading: (workType, state) => `${workType} · ${state}`,
    currentDispatchLabel: "現在のディスパッチ",
    dispatchLabel: "ディスパッチ",
    elapsedLabel: "経過時間",
    totalRuntimeLabel: "合計実行時間",
    expandAction: "展開",
    historyRequestCountLabel: (count) => `${count} 件のリクエスト`,
    historyRunCountLabel: (count) => `${count} 件のラン`,
    historicalRequestsLabel: "過去のリクエスト",
    historicalRunsLabel: "過去のラン",
    inputWorkTypesLabel: "入力ワークタイプ",
    kindDefaultValue: "STANDARD",
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
    requestStatusStartedAgo: (elapsed) => `${elapsed} に開始`,
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `有効な runner: ${runnerName}（${sourceLabel}）。`,
    editableConfigurationCronExpiryWindowInvalid: (value) =>
      `expiry_window must be a positive duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronJitterInvalid: (value) =>
      `jitter must be a non-negative duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronScheduleInvalid: (schedule, detail) =>
      `invalid cron schedule ${JSON.stringify(schedule)}: ${detail}`,
    editableConfigurationCronScheduleRequired:
      "cron workstation requires non-empty 'schedule'",
    cronExpiryWindowFieldHint:
      "期限切れ前の正の Go duration（例: 30s、5m、1h）。省略可。",
    cronExpiryWindowFieldLabel: "Cron 有効期限ウィンドウ",
    cronJitterFieldHint:
      "スケジュールに加える最大ジッターの非負 Go duration（例: 0s、30s、5m）。省略可。",
    cronJitterFieldLabel: "Cron ジッター",
    cronScheduleFieldHint: "必須の 5 フィールド cron 式（例: */5 * * * *）。",
    cronScheduleFieldLabel: "Cron スケジュール",
    cronTriggerAtStartFieldLabel: "Cron 起動時トリガー",
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `ファクトリー runner を継承 (${runnerName})`,
    runnerInheritanceFactoryMissingLabel: "既定 runner (Codex) を継承",
    runnerLoadingValue: "runner を読み込み中...",
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
    unavailableWorkstationKindValue: "ワークステーション種別は利用できません",
    unavailableWorkstationTypeValue: "ワークステーション種別は利用できません",
    unknownWorkerTypeValue: "不明",
    unknownWorkLabel: "不明な作業",
    workDetailsUnavailable: (dispatchId) =>
      `ディスパッチ ${dispatchId} の作業詳細は利用できません。`,
    workIdLabel: "ワーク ID",
    workSelectedAction: "ワークを選択済み",
    workerTypeLabel: "ワーカータイプ",
    workstationKindLoadingValue: "ワークステーション種別を読み込み中...",
    workstationTypeLabel: "ワークステーション種別",
    workstationTypeLoadingValue: "ワークステーション種別を読み込み中...",
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
    editableConfigurationModelSharedWorkerHint:
      "공유 워커에는 모델 편집이 비활성화됩니다.",
    editableConfigurationNameDuplicate: (workstationName) =>
      `워크스테이션 이름 "${workstationName}" 은(는) 실행 중인 팩토리 정의에 이미 있습니다.`,
    editableConfigurationNameRequired:
      "이 워크스테이션을 저장하기 전에 워크스테이션 이름을 입력하세요.",
    editableConfigurationResetAction: "최신값으로 재설정",
    editableConfigurationServerFieldChangedHint:
      "편집하는 동안 실행 중인 팩토리에서 이 필드가 변경되었습니다. 최신값으로 재설정하면 로컬 초안 값이 버려집니다.",
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
    editableConfigurationSaveStaleVersionDetail:
      "최신 실행 중 팩토리 값을 다시 불러오거나, 이 초안을 유지한 채 편집기가 새로고침된 뒤 다시 시도하세요.",
    editableConfigurationSaveSuccess: (workstationName) =>
      `실행 중인 팩토리를 저장했습니다. ${workstationName} 은(는) 실행 중 팩토리 정의에서 업데이트되었습니다.`,
    editableConfigurationLoading:
      "이 워크스테이션의 현재 팩토리 정의를 불러오는 중입니다.",
    editableConfigurationValidationStatus:
      "이 워크스테이션을 저장하기 전에 강조 표시된 필드를 수정하세요.",
    editableConfigurationBehaviorPollerHint:
      "폴러 워크스테이션은 장기 실행 인그레스 워커를 감독합니다. 호스티드 또는 스크립트 워커는 별도로 구성하고, 이 워크스테이션은 폴러 동작 바인딩과 출력 라우팅만 담당합니다.",
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
      "저장하기 전에 강조 표시된 문제를 수정하세요.",
    editableConfigurationPromptFieldHint: "아래 프롬프트 진단을 확인하세요.",
    editableConfigurationPromptDiagnosticsHeading: "프롬프트 진단",
    editableConfigurationPromptSyntaxDiagnosticLabel: "템플릿 구문",
    editableConfigurationPromptVariableDiagnosticLabel: "변수 접근",
    editableConfigurationPromptHelpLoading:
      "이 워크스테이션에서 사용할 수 있는 프롬프트 변수를 불러오는 중입니다.",
    editableConfigurationPromptHelpEmpty:
      "이 워크스테이션에는 사용할 수 있는 프롬프트 변수 도움말이 없습니다.",
    editableConfigurationPromptHelpFallbackError:
      "프롬프트 변수 도움말을 불러올 수 없습니다.",
    editableConfigurationPromptHelpErrorPrefix:
      "프롬프트 변수 도움말을 사용할 수 없습니다.",
    editableConfigurationPromptAutocompleteSummary: (
      variableCount,
      inputCount,
    ) =>
      `${inputCount}개의 입력 컨텍스트에서 ${variableCount}개의 자동완성 변수를 사용할 수 있습니다.`,
    editableConfigurationPromptAutocompleteDetail:
      "추천은 {{ ... }} 안에서 입력할 때만 표시됩니다.",
    editableConfigurationPromptHelpExpandActionLabel:
      "프롬프트 변수 도움말 열기",
    editableConfigurationPromptHelpCollapseActionLabel:
      "프롬프트 변수 도움말 닫기",
    editableConfigurationPromptAvailableVariablesHeading: "사용 가능한 변수",
    editableConfigurationPromptUnavailableAccessHeading: "사용할 수 없는 접근",
    editableConfigurationPromptResizeHandleLabel: "프롬프트 편집기 높이 조절",
    editableConfigurationSaveFallbackError:
      "실행 중인 팩토리를 저장할 수 없습니다.",
    editableConfigurationWorkerMissing:
      "선택한 워크스테이션이 현재 팩토리 정의에 더 이상 없는 워커를 참조합니다. 현재 선택을 다시 불러오고 다른 워커를 선택하세요.",
    editableConfigurationWorkerOptionsEmpty:
      "이 워크스테이션에서 사용할 수 있는 현재 워커가 없습니다. 이 필드를 편집하기 전에 팩토리에 워커를 추가하세요.",
    editableConfigurationWorkerRequired:
      "이 워크스테이션을 저장하기 전에 워커를 선택하세요.",
    editableConfigurationSharedWorkerScopeHint: (
      workerName,
      workstationNames,
    ) => `워커 ${workerName}는 ${workstationNames}에서도 사용됩니다.`,
    editableConfigurationWorkerUnavailable:
      "선택한 워커를 더 이상 사용할 수 없습니다. 저장하기 전에 다른 워커를 선택하세요.",
    editableConfigurationWorkerUnavailablePrefix:
      "워커 선택을 사용할 수 없습니다.",
    editableConfigurationModelInvokeBindingDuplicate: (slotName) =>
      `슬롯 "${slotName}"에 대한 작업 바인딩이 중복되었습니다.`,
    editableConfigurationModelInvokeBindingRequired: (slotName) =>
      `필수 슬롯 "${slotName}"에는 선택자, 구성 콘텐츠 또는 기본 콘텐츠가 필요합니다.`,
    editableConfigurationModelInvokeBindingsSummary:
      "저장하기 전에 강조된 작업 바인딩 항목을 해결하세요.",
    editableConfigurationModelInvokeOperationInvalid:
      "작업 이름은 대문자, 숫자, 밑줄만 사용할 수 있습니다.",
    editableConfigurationModelInvokeOperationMissing:
      "선택한 작업이 선택한 모델 워커에 선언되어 있지 않습니다.",
    editableConfigurationModelInvokeOperationOptionsEmpty:
      "선택한 모델 워커에 호환되는 작업이 선언되어 있지 않습니다.",
    editableConfigurationModelInvokeOperationRequired:
      "이 워크스테이션을 저장하기 전에 작업을 선택하세요.",
    editableConfigurationModelInvokeWorkerOptionsEmpty:
      "현재 팩토리 정의에 호환되는 작업을 가진 모델 워커가 없습니다.",
    editableConfigurationModelInvokeWorkerRequired:
      "작업을 선택하기 전에 모델 워커를 선택하세요.",
    modelInvokeBindingConfigContentFieldLabel: "구성 콘텐츠",
    modelInvokeBindingDefaultContentFieldLabel: "기본 콘텐츠",
    modelInvokeBindingsEmpty:
      "입력 슬롯 바인딩을 편집하려면 워커 작업을 선택하세요.",
    modelInvokeBindingsFieldHint:
      "바인딩은 선택자 필드, 정적 구성 콘텐츠 또는 기본 콘텐츠로 런타임 입력을 해석합니다. 선택 슬롯은 비워 두면 생략됩니다.",
    modelInvokeBindingsFieldLabel: "작업 바인딩",
    modelInvokeBindingOptionalSlotLabel: "선택",
    modelInvokeBindingRequiredSlotLabel: "필수",
    modelInvokeBindingSelectorLabelFieldLabel: "선택자 라벨",
    modelInvokeBindingSelectorRoleFieldLabel: "선택자 역할",
    modelInvokeBindingSelectorSlotFieldLabel: "선택자 슬롯",
    modelInvokeBindingSelectorTypeFieldLabel: "선택자 유형",
    modelInvokeBindingSelectorTypeNoneOption: "모든 유형",
    modelInvokeBindingSlotHeading: (slotName, requirement) =>
      `${slotName}(${requirement})`,
    modelInvokeOperationFieldLabel: "작업",
    editableConfigurationWorkstationOptionsEmpty:
      "현재 팩토리 정의에 사용 가능한 워크스테이션이 없습니다.",
    editableConfigurationWorkstationUnavailablePrefix:
      "워크스테이션 선택을 사용할 수 없습니다.",
    editableConfigurationVisitCountMaxVisitsInvalid:
      "최대 방문 횟수는 양의 정수여야 합니다.",
    editableConfigurationVisitCountWorkstationInvalid: (workstation) =>
      `카운트 대상 워크스테이션 ${workstation} 은(는) 이 팩토리에서 사용할 수 없습니다.`,
    editableConfigurationVisitCountWorkstationRequired:
      "방문 횟수를 셀 워크스테이션을 선택하세요.",
    editableConfigurationMatchesFieldsInputKeyRequired:
      "이 가드의 필드 선택자를 입력하세요.",
    editableConfigurationInputGuardMultipleGuards:
      "각 입력 슬롯에는 가드를 하나만 설정할 수 있습니다.",
    editableConfigurationInputGuardMatchInputRequired:
      "이 가드의 피어 입력을 선택하세요.",
    editableConfigurationInputGuardMatchInputInvalid: (workType) =>
      `피어 입력 ${workType} 은(는) 이 워크스테이션에서 사용할 수 없습니다.`,
    editableConfigurationInputGuardMatchInputSelfReference:
      "피어 입력은 같은 입력 슬롯을 참조할 수 없습니다.",
    editableConfigurationInputGuardParentInputRequired:
      "이 가드의 부모 입력을 선택하세요.",
    editableConfigurationInputGuardParentInputInvalid: (workType) =>
      `부모 입력 ${workType} 은(는) 이 워크스테이션에서 사용할 수 없습니다.`,
    editableConfigurationInputGuardParentInputSelfReference:
      "부모 입력은 같은 입력 슬롯을 참조할 수 없습니다.",
    editableConfigurationInputGuardSpawnedByInvalid: (workstation) =>
      `spawned-by 워크스테이션 ${workstation} 은(는) 이 팩토리에서 사용할 수 없습니다.`,
    matchesFieldsGuardInputKeyFieldLabel: "필드 선택자",
    editableConfigurationGuardSelectorEditorLoading:
      "필드 선택자 편집기를 시작하는 중입니다.",
    editableConfigurationGuardSelectorEditorError:
      "필드 선택자 편집기를 시작할 수 없습니다. 이 워크스테이션을 다시 로드한 후 다시 시도하세요.",
    modelFieldLabel: "모델",
    notConfiguredValue: "구성되지 않음",
    promptFieldLabel: "프롬프트",
    templateFieldLabel: "템플릿",
    workerFieldLabel: "워커",
    workstationNameFieldLabel: "워크스테이션 이름",
    workstationGuardsHeading: "워크스테이션 가드",
    workstationGuardsEmpty: "구성된 워크스테이션 가드가 없습니다.",
    workstationGuardsAddLabel: "가드 추가",
    workstationGuardsAddPlaceholder: "가드 유형 선택",
    workstationGuardsRemoveAction: "가드 제거",
    visitCountGuardWorkstationFieldLabel: "카운트 대상 워크스테이션",
    visitCountGuardMaxVisitsFieldLabel: "최대 방문 횟수",
    inputGuardMatchInputFieldLabel: "피어 입력",
    inputGuardParentInputFieldLabel: "부모 입력",
    inputGuardSpawnedByFieldLabel: "생성 주체(선택)",
    workstationInputGuardsHeading: "입력 가드",
    workstationInputGuardsEmpty: "이 워크스테이션에 작성된 입력이 없습니다.",
    workstationInputGuardTypeFieldLabel: "입력 가드",
    workstationInputGuardNoneOption: "없음",
    workstationInputGuardPeersEmpty:
      "피어 기반 가드를 구성하려면 이 워크스테이션에 다른 입력을 추가하세요.",
    workstationInputSlotHeading: (workType, state) => `${workType} · ${state}`,
    currentDispatchLabel: "현재 디스패치",
    dispatchLabel: "디스패치",
    elapsedLabel: "경과 시간",
    totalRuntimeLabel: "총 실행 시간",
    expandAction: "펼치기",
    historyRequestCountLabel: (count) => `${count}개 요청`,
    historyRunCountLabel: (count) => `${count}개 실행`,
    historicalRequestsLabel: "이전 요청",
    historicalRunsLabel: "이전 실행",
    inputWorkTypesLabel: "입력 작업 유형",
    kindDefaultValue: "STANDARD",
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
    requestStatusStartedAgo: (elapsed) => `${elapsed} 시작`,
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `유효 runner: ${runnerName} (${sourceLabel}).`,
    editableConfigurationCronExpiryWindowInvalid: (value) =>
      `expiry_window must be a positive duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronJitterInvalid: (value) =>
      `jitter must be a non-negative duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronScheduleInvalid: (schedule, detail) =>
      `invalid cron schedule ${JSON.stringify(schedule)}: ${detail}`,
    editableConfigurationCronScheduleRequired:
      "cron workstation requires non-empty 'schedule'",
    cronExpiryWindowFieldHint:
      "만료 전 양의 Go duration(예: 30s, 5m, 1h). 선택 사항.",
    cronExpiryWindowFieldLabel: "Cron 만료 윈도우",
    cronJitterFieldHint:
      "최대 결정적 지터의 비음 Go duration(예: 0s, 30s, 5m). 선택 사항.",
    cronJitterFieldLabel: "Cron 지터",
    cronScheduleFieldHint: "필수 5필드 cron 식(예: */5 * * * *).",
    cronScheduleFieldLabel: "Cron 스케줄",
    cronTriggerAtStartFieldLabel: "Cron 시작 시 트리거",
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `팩토리 runner 상속 (${runnerName})`,
    runnerInheritanceFactoryMissingLabel: "기본 runner (Codex) 상속",
    runnerLoadingValue: "runner 불러오는 중...",
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
    unavailableWorkstationKindValue: "워크스테이션 종류를 사용할 수 없음",
    unavailableWorkstationTypeValue: "워크스테이션 유형을 사용할 수 없음",
    unknownWorkerTypeValue: "알 수 없음",
    unknownWorkLabel: "알 수 없는 작업",
    workDetailsUnavailable: (dispatchId) =>
      `디스패치 ${dispatchId}의 작업 세부정보를 사용할 수 없습니다.`,
    workIdLabel: "작업 ID",
    workSelectedAction: "작업 선택됨",
    workerTypeLabel: "워커 유형",
    workstationKindLoadingValue: "워크스테이션 종류 불러오는 중...",
    workstationTypeLabel: "워크스테이션 유형",
    workstationTypeLoadingValue: "워크스테이션 유형 불러오는 중...",
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
    editableConfigurationModelSharedWorkerHint: "共享 worker 时不能编辑模型。",
    editableConfigurationNameDuplicate: (workstationName) =>
      `工作站名称 "${workstationName}" 在运行中的工厂定义中已存在。`,
    editableConfigurationNameRequired: "保存此工作站前请输入工作站名称。",
    editableConfigurationResetAction: "重置为最新值",
    editableConfigurationServerFieldChangedHint:
      "你编辑期间，运行中的工厂已更新此字段。重置为最新值会丢弃当前本地草稿值。",
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
    editableConfigurationSaveStaleVersionDetail:
      "请重新加载最新的运行中工厂值，或保留此草稿并在编辑器刷新后重试。",
    editableConfigurationSaveSuccess: (workstationName) =>
      `运行中的工厂已保存。${workstationName} 已在运行中的工厂定义中更新。`,
    editableConfigurationLoading: "正在加载此工作站的当前工厂定义。",
    editableConfigurationValidationStatus: "请先修正高亮字段，再保存此工作站。",
    editableConfigurationBehaviorPollerHint:
      "轮询器工作站会监督长期运行的入口工作者。请在工作者侧单独配置托管或脚本轮询器；此工作站只负责绑定轮询器行为并路由其输出。",
    editableConfigurationBehaviorPollerWorkerUnsupported:
      "保存此工作站前，请先为轮询器工作站选择脚本或 hosted worker。",
    editableConfigurationPromptRequired: "保存此工作站前请输入提示词。",
    editableConfigurationPromptEditorLoading:
      "正在启动此工作站的提示词编辑器。",
    editableConfigurationPromptEditorError:
      "无法启动提示词编辑器。请重新加载此工作站后重试。",
    editableConfigurationPromptValidationLoading:
      "正在校验当前草稿中的提示词变量。",
    editableConfigurationPromptValidationFallbackError: "无法完成提示词校验。",
    editableConfigurationPromptValidationErrorPrefix: "提示词校验不可用。",
    editableConfigurationPromptDiagnosticsSummary:
      "保存前请先修正高亮显示的问题。",
    editableConfigurationPromptFieldHint: "请查看下方的提示词诊断。",
    editableConfigurationPromptDiagnosticsHeading: "提示词诊断",
    editableConfigurationPromptSyntaxDiagnosticLabel: "模板语法",
    editableConfigurationPromptVariableDiagnosticLabel: "变量访问",
    editableConfigurationPromptHelpLoading:
      "正在加载此工作站可用的提示词变量。",
    editableConfigurationPromptHelpEmpty:
      "此工作站没有可用的提示词变量帮助信息。",
    editableConfigurationPromptHelpFallbackError:
      "无法加载提示词变量帮助信息。",
    editableConfigurationPromptHelpErrorPrefix: "提示词变量帮助信息不可用。",
    editableConfigurationPromptAutocompleteSummary: (
      variableCount,
      inputCount,
    ) =>
      `自动补全已就绪，可为 ${inputCount} 个已编写输入上下文提供 ${variableCount} 个变量候选。`,
    editableConfigurationPromptAutocompleteDetail:
      "仅在 {{ ... }} 内输入时显示建议。",
    editableConfigurationPromptHelpExpandActionLabel: "打开提示词变量帮助",
    editableConfigurationPromptHelpCollapseActionLabel: "关闭提示词变量帮助",
    editableConfigurationPromptAvailableVariablesHeading: "可用变量",
    editableConfigurationPromptUnavailableAccessHeading: "不可用访问",
    editableConfigurationPromptResizeHandleLabel: "调整提示词编辑器高度",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
    editableConfigurationWorkerMissing:
      "所选工作站引用的工作器已不在当前工厂定义中。请重新加载当前选择并选择其他工作器。",
    editableConfigurationWorkerOptionsEmpty:
      "此工作站当前没有可用的工作器。请先向工厂添加工作器，再编辑此字段。",
    editableConfigurationWorkerRequired: "保存此工作站前请选择工作器。",
    editableConfigurationSharedWorkerScopeHint: (
      workerName,
      workstationNames,
    ) => `工作器 ${workerName} 也被 ${workstationNames} 使用。`,
    editableConfigurationWorkerUnavailable:
      "所选工作器已不可用。保存前请选择其他工作器。",
    editableConfigurationWorkerUnavailablePrefix: "工作器选择不可用。",
    editableConfigurationModelInvokeBindingDuplicate: (slotName) =>
      `槽位 "${slotName}" 的操作绑定重复声明。`,
    editableConfigurationModelInvokeBindingRequired: (slotName) =>
      `必填槽位 "${slotName}" 需要选择器、配置内容或默认内容。`,
    editableConfigurationModelInvokeBindingsSummary:
      "保存前请解决高亮的操作绑定字段。",
    editableConfigurationModelInvokeOperationInvalid:
      "操作名称只能使用大写字母、数字或下划线。",
    editableConfigurationModelInvokeOperationMissing:
      "所选操作未在所选模型工作器上声明。",
    editableConfigurationModelInvokeOperationOptionsEmpty:
      "所选模型工作器未声明任何兼容操作。",
    editableConfigurationModelInvokeOperationRequired:
      "保存此工作站前请选择一个操作。",
    editableConfigurationModelInvokeWorkerOptionsEmpty:
      "当前工厂定义中没有声明兼容操作的模型工作器。",
    editableConfigurationModelInvokeWorkerRequired:
      "选择操作前请先选择模型工作器。",
    modelInvokeBindingConfigContentFieldLabel: "配置内容",
    modelInvokeBindingDefaultContentFieldLabel: "默认内容",
    modelInvokeBindingsEmpty: "请选择工作器操作以编辑输入槽位绑定。",
    modelInvokeBindingsFieldHint:
      "绑定通过选择器字段、静态配置内容或默认内容解析运行时输入。可选槽位留空即可省略。",
    modelInvokeBindingsFieldLabel: "操作绑定",
    modelInvokeBindingOptionalSlotLabel: "可选",
    modelInvokeBindingRequiredSlotLabel: "必填",
    modelInvokeBindingSelectorLabelFieldLabel: "选择器标签",
    modelInvokeBindingSelectorRoleFieldLabel: "选择器角色",
    modelInvokeBindingSelectorSlotFieldLabel: "选择器槽位",
    modelInvokeBindingSelectorTypeFieldLabel: "选择器类型",
    modelInvokeBindingSelectorTypeNoneOption: "任意类型",
    modelInvokeBindingSlotHeading: (slotName, requirement) =>
      `${slotName}（${requirement}）`,
    modelInvokeOperationFieldLabel: "操作",
    editableConfigurationWorkstationOptionsEmpty:
      "当前工厂定义中没有可用的工作站。",
    editableConfigurationWorkstationUnavailablePrefix: "工作站选择不可用。",
    editableConfigurationVisitCountMaxVisitsInvalid:
      "最大访问次数必须是正整数。",
    editableConfigurationVisitCountWorkstationInvalid: (workstation) =>
      `计数工作站 ${workstation} 在当前工厂中不可用。`,
    editableConfigurationVisitCountWorkstationRequired:
      "请选择要计数访问次数的工作站。",
    editableConfigurationMatchesFieldsInputKeyRequired:
      "请输入此守卫的字段选择器。",
    editableConfigurationInputGuardMultipleGuards:
      "每个输入槽最多只能配置一个守卫。",
    editableConfigurationInputGuardMatchInputRequired:
      "请为此守卫选择对等输入。",
    editableConfigurationInputGuardMatchInputInvalid: (workType) =>
      `对等输入 ${workType} 在此工作站上不可用。`,
    editableConfigurationInputGuardMatchInputSelfReference:
      "对等输入不能引用同一个输入槽。",
    editableConfigurationInputGuardParentInputRequired:
      "请为此守卫选择父输入。",
    editableConfigurationInputGuardParentInputInvalid: (workType) =>
      `父输入 ${workType} 在此工作站上不可用。`,
    editableConfigurationInputGuardParentInputSelfReference:
      "父输入不能引用同一个输入槽。",
    editableConfigurationInputGuardSpawnedByInvalid: (workstation) =>
      `spawned-by 工作站 ${workstation} 在当前工厂中不可用。`,
    matchesFieldsGuardInputKeyFieldLabel: "字段选择器",
    editableConfigurationGuardSelectorEditorLoading:
      "正在启动字段选择器编辑器。",
    editableConfigurationGuardSelectorEditorError:
      "无法启动字段选择器编辑器。请重新加载此工作站后重试。",
    modelFieldLabel: "模型",
    notConfiguredValue: "未配置",
    promptFieldLabel: "提示词",
    templateFieldLabel: "模板",
    workerFieldLabel: "工作器",
    workstationNameFieldLabel: "工作站名称",
    workstationGuardsHeading: "工作站守卫",
    workstationGuardsEmpty: "未配置工作站守卫。",
    workstationGuardsAddLabel: "添加守卫",
    workstationGuardsAddPlaceholder: "选择守卫类型",
    workstationGuardsRemoveAction: "移除守卫",
    visitCountGuardWorkstationFieldLabel: "计数工作站",
    visitCountGuardMaxVisitsFieldLabel: "最大访问次数",
    inputGuardMatchInputFieldLabel: "对等输入",
    inputGuardParentInputFieldLabel: "父输入",
    inputGuardSpawnedByFieldLabel: "生成方（可选）",
    workstationInputGuardsHeading: "输入守卫",
    workstationInputGuardsEmpty: "此工作站没有已编写的输入。",
    workstationInputGuardTypeFieldLabel: "输入守卫",
    workstationInputGuardNoneOption: "无",
    workstationInputGuardPeersEmpty:
      "请在此工作站添加另一个输入，以配置基于对等的守卫。",
    workstationInputSlotHeading: (workType, state) => `${workType} · ${state}`,
    currentDispatchLabel: "当前分派",
    dispatchLabel: "分派",
    elapsedLabel: "已用时间",
    totalRuntimeLabel: "总运行时间",
    expandAction: "展开",
    historyRequestCountLabel: (count) => `${count} 个请求`,
    historyRunCountLabel: (count) => `${count} 次运行`,
    historicalRequestsLabel: "历史请求",
    historicalRunsLabel: "历史运行",
    inputWorkTypesLabel: "输入工作类型",
    kindDefaultValue: "STANDARD",
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
    requestStatusStartedAgo: (elapsed) => `开始于 ${elapsed}`,
    runnerFieldHelp: (runnerName, sourceLabel) =>
      `当前生效的 runner：${runnerName}（${sourceLabel}）。`,
    editableConfigurationCronExpiryWindowInvalid: (value) =>
      `expiry_window must be a positive duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronJitterInvalid: (value) =>
      `jitter must be a non-negative duration, got ${JSON.stringify(value)}`,
    editableConfigurationCronScheduleInvalid: (schedule, detail) =>
      `invalid cron schedule ${JSON.stringify(schedule)}: ${detail}`,
    editableConfigurationCronScheduleRequired:
      "cron workstation requires non-empty 'schedule'",
    cronExpiryWindowFieldHint:
      "可选的正 Go duration，表示到期后过期前的窗口（例如 30s、5m、1h）。",
    cronExpiryWindowFieldLabel: "Cron 过期窗口",
    cronJitterFieldHint:
      "可选的非负 Go duration，表示最大确定性调度抖动（例如 0s、30s、5m）。",
    cronJitterFieldLabel: "Cron 抖动",
    cronScheduleFieldHint: "必填的五字段 cron 表达式（例如 */5 * * * *）。",
    cronScheduleFieldLabel: "Cron 调度",
    cronTriggerAtStartFieldLabel: "Cron 启动时触发",
    runnerFieldLabel: "Runner",
    runnerInheritanceFactoryLabel: (runnerName) =>
      `继承工厂 runner（${runnerName}）`,
    runnerInheritanceFactoryMissingLabel: "继承默认 runner（Codex）",
    runnerLoadingValue: "正在加载 runner...",
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
    unavailableWorkstationKindValue: "工作站种类不可用",
    unavailableWorkstationTypeValue: "工作站类型不可用",
    unknownWorkerTypeValue: "未知",
    unknownWorkLabel: "未知工作",
    workDetailsUnavailable: (dispatchId) =>
      `无法提供分派 ${dispatchId} 的工作详情。`,
    workIdLabel: "工作 ID",
    workSelectedAction: "工作已选中",
    workerTypeLabel: "工作器类型",
    workstationKindLoadingValue: "正在加载工作站种类...",
    workstationTypeLabel: "工作站类型",
    workstationTypeLoadingValue: "正在加载工作站类型...",
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
    localizeProviderSessionKind: enumMessages.localizeProviderSessionKind,
    localizeRunnerSelectionSource: enumMessages.localizeRunnerSelectionSource,
    localizeWorkstationBehavior: enumMessages.localizeWorkstationBehavior,
    localizeWorkstationGuardType: enumMessages.localizeWorkstationGuardType,
    localizeInputGuardType: enumMessages.localizeInputGuardType,
    localizeWorkstationKind: enumMessages.localizeWorkstationKind,
    localizeWorkstationType: enumMessages.localizeWorkstationType,
  };
}

export type { WorkstationDetailMessages } from "./workstation-detail-types";
export { workstationDetailMessagesByLocale };
