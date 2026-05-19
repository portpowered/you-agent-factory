import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CurrentSelectionDispatchHistoryMessages {
  awaitingProviderResponse: string;
  collapseAction: string;
  commandLabel: string;
  currentDispatchBadge: string;
  completedAttemptLabel: string;
  durationLabel: string;
  exitCodeLabel: string;
  expandAction: string;
  failureDetailsTitle: string;
  failureMessageLabel: string;
  failureReasonLabel: string;
  failureTypeLabel: string;
  inferenceAttemptsTitle: string;
  inferenceAttemptsEmptyEnded: string;
  inferenceAttemptsEmptyPending: string;
  inferenceAttemptAccessibleLabel: (attemptNumber: number) => string;
  inferenceAttemptLabel: (attemptNumber: number) => string;
  inferenceRequestGuidance: string;
  inputWorkLabel: string;
  noScriptResponseYet: string;
  noScriptAttemptRecordedYet: string;
  noStderrRecorded: string;
  noStdoutRecorded: string;
  outputWorkLabel: string;
  outcomeLabel: string;
  pendingAttemptLabel: string;
  pendingOutcome: string;
  providerLabel: string;
  providerResponseUnavailable: string;
  providerSessionLabel: string;
  recordedAttemptStatus: string;
  promptDetailsNotApplicable: string;
  requestDetailsTitle: string;
  requestAttemptLabel: (attemptLabel: string) => string;
  resolvedArgsLabel: string;
  selectedTraceSuffix: string;
  selectWorkItemAccessibleLabel: (workItemLabel: string) => string;
  modelLabel: string;
  responseDetailsTitle: string;
  responseAttemptLabel: (attemptLabel: string) => string;
  scriptAttemptLabel: string;
  scriptAttemptsTitle: string;
  scriptRequestIdLabel: string;
  scriptRequestPlaceholderId: string;
  scriptResponsePlaceholderId: string;
  startedAtLabel: string;
  stderrLabel: string;
  stdoutLabel: string;
  traceDetailsTitle: string;
  traceIdsLabel: string;
  transitionIdLabel: string;
  unknownDispatchId: string;
  unknownDispatchTitle: string;
  openWorkItemActionLabel: (workItemLabel: string) => string;
  workstationLabel: string;
  workingDirectoryLabel: string;
  workSelectedActionLabel: string;
  worktreeLabel: string;
}

const currentSelectionDispatchHistoryMessagesByLocale = {
  en: {
    awaitingProviderResponse: "Awaiting provider response.",
    collapseAction: "Collapse",
    commandLabel: "Command",
    completedAttemptLabel: "completed",
    currentDispatchBadge: "Current dispatch",
    durationLabel: "Duration",
    exitCodeLabel: "Exit code",
    expandAction: "Expand",
    failureDetailsTitle: "Failure details",
    failureMessageLabel: "Failure message",
    failureReasonLabel: "Failure reason",
    failureTypeLabel: "Failure type",
    inferenceAttemptsTitle: "Inference attempts",
    inferenceAttemptsEmptyEnded:
      "No inference attempt details were recorded before this dispatch ended.",
    inferenceAttemptsEmptyPending:
      "No inference attempt details have been recorded for this dispatch yet.",
    inferenceAttemptAccessibleLabel: (attemptNumber: number) =>
      `Inference attempt ${attemptNumber}`,
    inferenceAttemptLabel: (attemptNumber: number) => `Attempt ${attemptNumber}`,
    inferenceRequestGuidance:
      "Inference request details are shown under Inference attempts.",
    inputWorkLabel: "Input work",
    noScriptResponseYet: "No script response yet for this dispatch.",
    noScriptAttemptRecordedYet: "No script response attempt has been recorded yet.",
    noStderrRecorded: "No stderr was recorded for this script response.",
    noStdoutRecorded: "No stdout was recorded for this script response.",
    outputWorkLabel: "Output work",
    outcomeLabel: "Outcome",
    pendingAttemptLabel: "pending",
    pendingOutcome: "PENDING",
    providerLabel: "Provider",
    providerResponseUnavailable:
      "Provider response text is not available for this inference attempt.",
    providerSessionLabel: "providerSession",
    recordedAttemptStatus: "RECORDED",
    promptDetailsNotApplicable:
      "Prompt details are not applicable to this script-backed dispatch.",
    requestDetailsTitle: "Request details",
    requestAttemptLabel: (attemptLabel: string) =>
      `Request attempt ${attemptLabel}`,
    resolvedArgsLabel: "Resolved args",
    selectedTraceSuffix: " (selected)",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `Select work item ${workItemLabel}`,
    modelLabel: "Model",
    responseDetailsTitle: "Response details",
    responseAttemptLabel: (attemptLabel: string) =>
      `Response attempt ${attemptLabel}`,
    scriptAttemptLabel: "Script attempt",
    scriptAttemptsTitle: "Script attempts",
    scriptRequestIdLabel: "Script request ID",
    scriptRequestPlaceholderId: "script-request",
    scriptResponsePlaceholderId: "script-response",
    startedAtLabel: "Started at",
    stderrLabel: "Stderr",
    stdoutLabel: "Stdout",
    traceDetailsTitle: "Trace details",
    traceIdsLabel: "Trace IDs",
    transitionIdLabel: "Transition ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "Unknown dispatch",
    openWorkItemActionLabel: (workItemLabel: string) => `Open ${workItemLabel}`,
    workstationLabel: "Workstation",
    workingDirectoryLabel: "Working directory",
    workSelectedActionLabel: "Work selected",
    worktreeLabel: "Worktree",
  },
  ja: {
    awaitingProviderResponse: "プロバイダー応答を待機しています。",
    collapseAction: "折りたたむ",
    commandLabel: "コマンド",
    completedAttemptLabel: "完了済み",
    currentDispatchBadge: "現在のディスパッチ",
    durationLabel: "所要時間",
    exitCodeLabel: "終了コード",
    expandAction: "展開",
    failureDetailsTitle: "失敗の詳細",
    failureMessageLabel: "失敗メッセージ",
    failureReasonLabel: "失敗理由",
    failureTypeLabel: "失敗タイプ",
    inferenceAttemptsTitle: "推論試行",
    inferenceAttemptsEmptyEnded:
      "このディスパッチが終了するまでに推論試行の詳細は記録されませんでした。",
    inferenceAttemptsEmptyPending:
      "このディスパッチの推論試行の詳細はまだ記録されていません。",
    inferenceAttemptAccessibleLabel: (attemptNumber: number) =>
      `推論試行 ${attemptNumber}`,
    inferenceAttemptLabel: (attemptNumber: number) => `試行 ${attemptNumber}`,
    inferenceRequestGuidance:
      "推論リクエストの詳細は推論試行の下に表示されます。",
    inputWorkLabel: "入力作業",
    noScriptResponseYet: "このディスパッチにはまだスクリプト応答がありません。",
    noScriptAttemptRecordedYet:
      "スクリプト応答の試行はまだ記録されていません。",
    noStderrRecorded: "このスクリプト応答では stderr は記録されませんでした。",
    noStdoutRecorded: "このスクリプト応答では stdout は記録されませんでした。",
    outputWorkLabel: "出力作業",
    outcomeLabel: "結果",
    pendingAttemptLabel: "保留中",
    pendingOutcome: "保留中",
    providerLabel: "プロバイダー",
    providerResponseUnavailable:
      "この推論試行ではプロバイダー応答テキストを利用できません。",
    providerSessionLabel: "providerSession",
    recordedAttemptStatus: "記録済み",
    promptDetailsNotApplicable:
      "このスクリプトベースのディスパッチではプロンプトの詳細は適用されません。",
    requestDetailsTitle: "リクエストの詳細",
    requestAttemptLabel: (attemptLabel: string) =>
      `リクエスト試行 ${attemptLabel}`,
    resolvedArgsLabel: "解決済み引数",
    selectedTraceSuffix: "（選択中）",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `作業項目 ${workItemLabel} を選択`,
    modelLabel: "モデル",
    responseDetailsTitle: "応答の詳細",
    responseAttemptLabel: (attemptLabel: string) =>
      `応答試行 ${attemptLabel}`,
    scriptAttemptLabel: "スクリプト試行",
    scriptAttemptsTitle: "スクリプト試行",
    scriptRequestIdLabel: "スクリプトリクエスト ID",
    scriptRequestPlaceholderId: "script-request",
    scriptResponsePlaceholderId: "script-response",
    startedAtLabel: "開始時刻",
    stderrLabel: "標準エラー",
    stdoutLabel: "標準出力",
    traceDetailsTitle: "トレースの詳細",
    traceIdsLabel: "トレース ID",
    transitionIdLabel: "遷移 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "不明なディスパッチ",
    openWorkItemActionLabel: (workItemLabel: string) =>
      `${workItemLabel} を開く`,
    workstationLabel: "ワークステーション",
    workingDirectoryLabel: "作業ディレクトリ",
    workSelectedActionLabel: "作業を選択中",
    worktreeLabel: "ワークツリー",
  },
  ko: {
    awaitingProviderResponse: "공급자 응답을 기다리는 중입니다.",
    collapseAction: "접기",
    commandLabel: "명령",
    completedAttemptLabel: "완료됨",
    currentDispatchBadge: "현재 디스패치",
    durationLabel: "소요 시간",
    exitCodeLabel: "종료 코드",
    expandAction: "펼치기",
    failureDetailsTitle: "실패 세부 정보",
    failureMessageLabel: "실패 메시지",
    failureReasonLabel: "실패 원인",
    failureTypeLabel: "실패 유형",
    inferenceAttemptsTitle: "추론 시도",
    inferenceAttemptsEmptyEnded:
      "이 디스패치가 끝나기 전까지 추론 시도 세부 정보가 기록되지 않았습니다.",
    inferenceAttemptsEmptyPending:
      "이 디스패치의 추론 시도 세부 정보가 아직 기록되지 않았습니다.",
    inferenceAttemptAccessibleLabel: (attemptNumber: number) =>
      `추론 시도 ${attemptNumber}`,
    inferenceAttemptLabel: (attemptNumber: number) => `시도 ${attemptNumber}`,
    inferenceRequestGuidance:
      "추론 요청 세부 정보는 추론 시도 아래에 표시됩니다.",
    inputWorkLabel: "입력 작업",
    noScriptResponseYet: "이 디스패치에는 아직 스크립트 응답이 없습니다.",
    noScriptAttemptRecordedYet: "아직 기록된 스크립트 응답 시도가 없습니다.",
    noStderrRecorded: "이 스크립트 응답에는 stderr가 기록되지 않았습니다.",
    noStdoutRecorded: "이 스크립트 응답에는 stdout이 기록되지 않았습니다.",
    outputWorkLabel: "출력 작업",
    outcomeLabel: "결과",
    pendingAttemptLabel: "대기 중",
    pendingOutcome: "대기 중",
    providerLabel: "공급자",
    providerResponseUnavailable:
      "이 추론 시도에 대한 공급자 응답 텍스트를 사용할 수 없습니다.",
    providerSessionLabel: "providerSession",
    recordedAttemptStatus: "기록됨",
    promptDetailsNotApplicable:
      "이 스크립트 기반 디스패치에는 프롬프트 세부 정보를 적용할 수 없습니다.",
    requestDetailsTitle: "요청 세부 정보",
    requestAttemptLabel: (attemptLabel: string) => `요청 시도 ${attemptLabel}`,
    resolvedArgsLabel: "해결된 인수",
    selectedTraceSuffix: " (선택됨)",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `작업 항목 ${workItemLabel} 선택`,
    modelLabel: "모델",
    responseDetailsTitle: "응답 세부 정보",
    responseAttemptLabel: (attemptLabel: string) => `응답 시도 ${attemptLabel}`,
    scriptAttemptLabel: "스크립트 시도",
    scriptAttemptsTitle: "스크립트 시도",
    scriptRequestIdLabel: "스크립트 요청 ID",
    scriptRequestPlaceholderId: "script-request",
    scriptResponsePlaceholderId: "script-response",
    startedAtLabel: "시작 시각",
    stderrLabel: "표준 오류",
    stdoutLabel: "표준 출력",
    traceDetailsTitle: "추적 세부 정보",
    traceIdsLabel: "추적 ID",
    transitionIdLabel: "전환 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "알 수 없는 디스패치",
    openWorkItemActionLabel: (workItemLabel: string) => `${workItemLabel} 열기`,
    workstationLabel: "워크스테이션",
    workingDirectoryLabel: "작업 디렉터리",
    workSelectedActionLabel: "작업 선택됨",
    worktreeLabel: "워크트리",
  },
  "zh-CN": {
    awaitingProviderResponse: "正在等待提供方响应。",
    collapseAction: "收起",
    commandLabel: "命令",
    completedAttemptLabel: "已完成",
    currentDispatchBadge: "当前分派",
    durationLabel: "耗时",
    exitCodeLabel: "退出码",
    expandAction: "展开",
    failureDetailsTitle: "失败详情",
    failureMessageLabel: "失败消息",
    failureReasonLabel: "失败原因",
    failureTypeLabel: "失败类型",
    inferenceAttemptsTitle: "推理尝试",
    inferenceAttemptsEmptyEnded: "该分派结束前没有记录任何推理尝试详情。",
    inferenceAttemptsEmptyPending: "该分派暂时还没有记录推理尝试详情。",
    inferenceAttemptAccessibleLabel: (attemptNumber: number) =>
      `推理尝试 ${attemptNumber}`,
    inferenceAttemptLabel: (attemptNumber: number) => `尝试 ${attemptNumber}`,
    inferenceRequestGuidance: "推理请求详情显示在推理尝试下方。",
    inputWorkLabel: "输入工作",
    noScriptResponseYet: "这个分派暂时还没有脚本响应。",
    noScriptAttemptRecordedYet: "这个分派暂时还没有记录脚本响应尝试。",
    noStderrRecorded: "这个脚本响应没有记录 stderr。",
    noStdoutRecorded: "这个脚本响应没有记录 stdout。",
    outputWorkLabel: "输出工作",
    outcomeLabel: "结果",
    pendingAttemptLabel: "等待中",
    pendingOutcome: "等待中",
    providerLabel: "提供方",
    providerResponseUnavailable: "此推理尝试的提供方响应文本不可用。",
    providerSessionLabel: "providerSession",
    recordedAttemptStatus: "已记录",
    promptDetailsNotApplicable: "这个脚本分派不适用提示词详情。",
    requestDetailsTitle: "请求详情",
    requestAttemptLabel: (attemptLabel: string) => `请求尝试 ${attemptLabel}`,
    resolvedArgsLabel: "已解析参数",
    selectedTraceSuffix: "（已选中）",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `选择工作项 ${workItemLabel}`,
    modelLabel: "模型",
    responseDetailsTitle: "响应详情",
    responseAttemptLabel: (attemptLabel: string) => `响应尝试 ${attemptLabel}`,
    scriptAttemptLabel: "脚本尝试",
    scriptAttemptsTitle: "脚本尝试",
    scriptRequestIdLabel: "脚本请求 ID",
    scriptRequestPlaceholderId: "script-request",
    scriptResponsePlaceholderId: "script-response",
    startedAtLabel: "开始时间",
    stderrLabel: "标准错误",
    stdoutLabel: "标准输出",
    traceDetailsTitle: "追踪详情",
    traceIdsLabel: "追踪 ID",
    transitionIdLabel: "转换 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "未知分派",
    openWorkItemActionLabel: (workItemLabel: string) => `打开 ${workItemLabel}`,
    workstationLabel: "工作站",
    workingDirectoryLabel: "工作目录",
    workSelectedActionLabel: "已选中工作项",
    worktreeLabel: "工作树",
  },
} satisfies LocalizedMessages<CurrentSelectionDispatchHistoryMessages>;

export function getCurrentSelectionDispatchHistoryMessages(
  locale?: string | null,
): CurrentSelectionDispatchHistoryMessages {
  return resolveLocalizedMessages(
    currentSelectionDispatchHistoryMessagesByLocale,
    locale,
  );
}

export { currentSelectionDispatchHistoryMessagesByLocale };
