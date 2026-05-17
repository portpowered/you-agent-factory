import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkstationDetailMessages {
  activeRunsLabel: string;
  activeWorkEmpty: string;
  activeWorkHeading: string;
  collapseAction: string;
  editableConfigurationEmpty: string;
  editableConfigurationErrorPrefix: string;
  editableConfigurationHeading: string;
  editableConfigurationLoading: string;
  editableConfigurationDirtyStatus: string;
  editableConfigurationDraftNote: string;
  editableConfigurationSummary: string;
  editableConfigurationValidationStatus: string;
  modelFieldLabel: string;
  notConfiguredValue: string;
  promptFieldLabel: string;
  templateFieldLabel: string;
  workerFieldLabel: string;
  currentDispatchLabel: string;
  dispatchLabel: string;
  elapsedLabel: string;
  expandAction: string;
  historyRequestCountLabel: (count: number) => string;
  historyRunCountLabel: (count: number) => string;
  historicalRequestsLabel: string;
  historicalRunsLabel: string;
  inputWorkTypesLabel: string;
  kindDefaultValue: string;
  kindLabel: string;
  noWorkstationRequests: string;
  noWorkstationRuns: string;
  openRequestAction: string;
  openRequestDetailsAction: string;
  openNamedWorkItemAction: (workItemLabel: string) => string;
  openWorkItemAction: string;
  outputWorkTypesLabel: string;
  projectedWorkstationRequestSummary: string;
  providerSummary: (provider: string, model?: string | null) => string;
  requestDetailsUnavailable: (dispatchId: string) => string;
  requestHistoryHeading: string;
  requestSelectedAction: string;
  requestStatusStartedAgo: (elapsed: string) => string;
  runHistoryHeading: string;
  providerSessionLogAction: string;
  providerSessionLogUnavailable: string;
  scriptCommandSummary: (command: string) => string;
  selectRequestLabel: (requestLabel: string, dispatchId: string) => string;
  selectWorkItemLabel: (workItemLabel: string) => string;
  selectWorkstationRequestLabel: (dispatchId: string) => string;
  selectedRequestLabel: (dispatchId: string) => string;
  stationLabel: string;
  summaryHeading: string;
  traceIdLabel: string;
  unknownActiveWorkLabel: string;
  unavailableValue: string;
  unknownWorkerTypeValue: string;
  unknownWorkLabel: string;
  workDetailsUnavailable: (dispatchId: string) => string;
  workIdLabel: string;
  workSelectedAction: string;
  workerTypeLabel: string;
}

const singularPlural = (count: number, singular: string, plural: string) =>
  `${count} ${count === 1 ? singular : plural}`;

const workstationDetailMessagesByLocale = {
  en: {
    activeRunsLabel: "Active runs",
    activeWorkEmpty: "No active work is running on this workstation.",
    activeWorkHeading: "Active work",
    collapseAction: "Collapse",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable prompt, model, and template values for the selected workstation.",
    editableConfigurationErrorPrefix: "Editable configuration unavailable.",
    editableConfigurationHeading: "Editable configuration",
    editableConfigurationDirtyStatus:
      "You have unsaved changes for this workstation.",
    editableConfigurationDraftNote:
      "Changes stay local to this edit session until you save the running factory.",
    editableConfigurationLoading:
      "Loading the current factory definition for this workstation.",
    editableConfigurationSummary:
      "Prompt, model, and template values are loaded from the latest editable current-factory definition.",
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this workstation.",
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
    runHistoryHeading: "Run history",
    providerSessionLogAction: "Codex session log",
    providerSessionLogUnavailable: "Session log unavailable",
    scriptCommandSummary: (command) => `Script command ${command}`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `Select request ${requestLabel} (${dispatchId})`,
    selectWorkItemLabel: (workItemLabel) => `Select work item ${workItemLabel}`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `Select workstation request ${dispatchId}`,
    selectedRequestLabel: (dispatchId) => `Selected request: ${dispatchId}.`,
    stationLabel: "Station",
    summaryHeading: "Workstation summary",
    traceIdLabel: "Trace ID",
    unknownActiveWorkLabel: "Unknown active work",
    unavailableValue: "Unavailable",
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
      "この選択中ワークステーションでは、実行中ファクトリー定義から編集可能な prompt、model、template の値を取得できません。",
    editableConfigurationErrorPrefix: "編集可能な構成は利用できません。",
    editableConfigurationHeading: "編集可能な構成",
    editableConfigurationDirtyStatus:
      "このワークステーションには未保存の変更があります。",
    editableConfigurationDraftNote:
      "変更は、実行中ファクトリーを保存するまでこの編集セッション内だけに保持されます。",
    editableConfigurationLoading:
      "このワークステーション向けに現在のファクトリー定義を読み込んでいます。",
    editableConfigurationSummary:
      "Prompt、model、template の値は最新の編集可能な current-factory 定義から読み込まれます。",
    editableConfigurationValidationStatus:
      "このワークステーションを保存する前に、強調表示された項目を修正してください。",
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
    runHistoryHeading: "ラン履歴",
    providerSessionLogAction: "Codex セッションログ",
    providerSessionLogUnavailable: "セッションログは利用できません",
    scriptCommandSummary: (command) => `スクリプトコマンド ${command}`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `リクエスト ${requestLabel} (${dispatchId}) を選択`,
    selectWorkItemLabel: (workItemLabel) =>
      `ワークアイテム ${workItemLabel} を選択`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `ワークステーションリクエスト ${dispatchId} を選択`,
    selectedRequestLabel: (dispatchId) => `選択中のリクエスト: ${dispatchId}。`,
    stationLabel: "ステーション",
    summaryHeading: "ワークステーション概要",
    traceIdLabel: "トレース ID",
    unknownActiveWorkLabel: "不明なアクティブ作業",
    unavailableValue: "利用不可",
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
      "선택한 워크스테이션에 대해 실행 중인 팩토리 정의에서 편집 가능한 prompt, model, template 값을 찾을 수 없습니다.",
    editableConfigurationErrorPrefix: "편집 가능한 구성을 사용할 수 없습니다.",
    editableConfigurationHeading: "편집 가능한 구성",
    editableConfigurationDirtyStatus:
      "이 워크스테이션에 저장되지 않은 변경 사항이 있습니다.",
    editableConfigurationDraftNote:
      "변경 사항은 실행 중인 팩토리를 저장할 때까지 이 편집 세션에만 로컬로 유지됩니다.",
    editableConfigurationLoading:
      "이 워크스테이션의 현재 팩토리 정의를 불러오는 중입니다.",
    editableConfigurationSummary:
      "Prompt, model, template 값은 최신 편집 가능한 current-factory 정의에서 로드됩니다.",
    editableConfigurationValidationStatus:
      "이 워크스테이션을 저장하기 전에 강조 표시된 필드를 수정하세요.",
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
    runHistoryHeading: "실행 기록",
    providerSessionLogAction: "Codex 세션 로그",
    providerSessionLogUnavailable: "세션 로그를 사용할 수 없음",
    scriptCommandSummary: (command) => `스크립트 명령 ${command}`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `요청 ${requestLabel} (${dispatchId}) 선택`,
    selectWorkItemLabel: (workItemLabel) => `작업 항목 ${workItemLabel} 선택`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `워크스테이션 요청 ${dispatchId} 선택`,
    selectedRequestLabel: (dispatchId) => `선택된 요청: ${dispatchId}.`,
    stationLabel: "스테이션",
    summaryHeading: "워크스테이션 요약",
    traceIdLabel: "추적 ID",
    unknownActiveWorkLabel: "알 수 없는 활성 작업",
    unavailableValue: "사용할 수 없음",
    unknownWorkerTypeValue: "알 수 없음",
    unknownWorkLabel: "알 수 없는 작업",
    workDetailsUnavailable: (dispatchId) =>
      `디스패치 ${dispatchId}의 작업 세부정보를 사용할 수 없습니다.`,
    workIdLabel: "작업 ID",
    workSelectedAction: "작업 선택됨",
    workerTypeLabel: "워커 유형",
  },
  zh: {
    activeRunsLabel: "活动运行",
    activeWorkEmpty: "此工作站当前没有正在运行的活动工作。",
    activeWorkHeading: "活动工作",
    collapseAction: "收起",
    editableConfigurationEmpty:
      "运行中的工厂定义没有为所选工作站公开可编辑的 prompt、model 和 template 值。",
    editableConfigurationErrorPrefix: "无法提供可编辑配置。",
    editableConfigurationHeading: "可编辑配置",
    editableConfigurationDirtyStatus: "此工作站存在未保存的更改。",
    editableConfigurationDraftNote:
      "在保存运行中的工厂之前，更改只会保留在当前编辑会话中。",
    editableConfigurationLoading: "正在加载此工作站的当前工厂定义。",
    editableConfigurationSummary:
      "Prompt、model 和 template 值来自最新可编辑的 current-factory 定义。",
    editableConfigurationValidationStatus: "请先修正高亮字段，再保存此工作站。",
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
    runHistoryHeading: "运行历史",
    providerSessionLogAction: "Codex 会话日志",
    providerSessionLogUnavailable: "会话日志不可用",
    scriptCommandSummary: (command) => `脚本命令 ${command}`,
    selectRequestLabel: (requestLabel, dispatchId) =>
      `选择请求 ${requestLabel} (${dispatchId})`,
    selectWorkItemLabel: (workItemLabel) => `选择工作项 ${workItemLabel}`,
    selectWorkstationRequestLabel: (dispatchId) =>
      `选择工作站请求 ${dispatchId}`,
    selectedRequestLabel: (dispatchId) => `已选择请求：${dispatchId}。`,
    stationLabel: "站点",
    summaryHeading: "工作站摘要",
    traceIdLabel: "跟踪 ID",
    unknownActiveWorkLabel: "未知活动工作",
    unavailableValue: "不可用",
    unknownWorkerTypeValue: "未知",
    unknownWorkLabel: "未知工作",
    workDetailsUnavailable: (dispatchId) =>
      `无法提供分派 ${dispatchId} 的工作详情。`,
    workIdLabel: "工作 ID",
    workSelectedAction: "工作已选中",
    workerTypeLabel: "工作器类型",
  },
} satisfies LocalizedMessages<WorkstationDetailMessages>;

export function getWorkstationDetailMessages(
  locale?: string | null,
): WorkstationDetailMessages {
  return resolveLocalizedMessages(workstationDetailMessagesByLocale, locale);
}

export { workstationDetailMessagesByLocale };
