import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CurrentSelectionDispatchHistoryMessages {
  commandLabel: string;
  currentDispatchBadge: string;
  dispatchedCountLabel: string;
  durationLabel: string;
  erroredCountLabel: string;
  exitCodeLabel: string;
  failureDetailsTitle: string;
  failureMessageLabel: string;
  failureReasonLabel: string;
  failureTypeLabel: string;
  inferenceAttemptsEmptyEnded: string;
  inferenceAttemptsEmptyPending: string;
  inferenceRequestGuidance: string;
  inputWorkLabel: string;
  noScriptResponseYet: string;
  noStderrRecorded: string;
  noStdoutRecorded: string;
  outputWorkLabel: string;
  outcomeLabel: string;
  pendingOutcome: string;
  promptDetailsNotApplicable: string;
  requestDetailsTitle: string;
  resolvedArgsLabel: string;
  respondedCountLabel: string;
  selectedTraceSuffix: string;
  selectWorkItemAccessibleLabel: (workItemLabel: string) => string;
  responseDetailsTitle: string;
  scriptAttemptLabel: string;
  scriptRequestIdLabel: string;
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
  workSelectedActionLabel: string;
}

const currentSelectionDispatchHistoryMessagesByLocale = {
  en: {
    commandLabel: "Command",
    currentDispatchBadge: "Current dispatch",
    dispatchedCountLabel: "dispatchedCount",
    durationLabel: "Duration",
    erroredCountLabel: "erroredCount",
    exitCodeLabel: "Exit code",
    failureDetailsTitle: "Failure details",
    failureMessageLabel: "Failure message",
    failureReasonLabel: "Failure reason",
    failureTypeLabel: "Failure type",
    inferenceAttemptsEmptyEnded:
      "No inference attempt details were recorded before this dispatch ended.",
    inferenceAttemptsEmptyPending:
      "No inference attempt details have been recorded for this dispatch yet.",
    inferenceRequestGuidance:
      "Inference request details are shown under Inference attempts.",
    inputWorkLabel: "Input work",
    noScriptResponseYet: "No script response yet for this dispatch.",
    noStderrRecorded: "No stderr was recorded for this script response.",
    noStdoutRecorded: "No stdout was recorded for this script response.",
    outputWorkLabel: "Output work",
    outcomeLabel: "Outcome",
    pendingOutcome: "PENDING",
    promptDetailsNotApplicable:
      "Prompt details are not applicable to this script-backed dispatch.",
    requestDetailsTitle: "Request details",
    resolvedArgsLabel: "Resolved args",
    respondedCountLabel: "respondedCount",
    selectedTraceSuffix: " (selected)",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `Select work item ${workItemLabel}`,
    responseDetailsTitle: "Response details",
    scriptAttemptLabel: "Script attempt",
    scriptRequestIdLabel: "Script request ID",
    startedAtLabel: "Started at",
    stderrLabel: "Stderr",
    stdoutLabel: "Stdout",
    traceDetailsTitle: "Trace details",
    traceIdsLabel: "Trace IDs",
    transitionIdLabel: "Transition ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "Unknown dispatch",
    openWorkItemActionLabel: (workItemLabel: string) =>
      `Open ${workItemLabel}`,
    workstationLabel: "Workstation",
    workSelectedActionLabel: "Work selected",
  },
  ja: {
    commandLabel: "コマンド",
    currentDispatchBadge: "現在のディスパッチ",
    dispatchedCountLabel: "ディスパッチ数",
    durationLabel: "所要時間",
    erroredCountLabel: "エラー数",
    exitCodeLabel: "終了コード",
    failureDetailsTitle: "失敗の詳細",
    failureMessageLabel: "失敗メッセージ",
    failureReasonLabel: "失敗理由",
    failureTypeLabel: "失敗タイプ",
    inferenceAttemptsEmptyEnded:
      "このディスパッチが終了するまでに推論試行の詳細は記録されませんでした。",
    inferenceAttemptsEmptyPending:
      "このディスパッチの推論試行の詳細はまだ記録されていません。",
    inferenceRequestGuidance:
      "推論リクエストの詳細は推論試行の下に表示されます。",
    inputWorkLabel: "入力作業",
    noScriptResponseYet: "このディスパッチにはまだスクリプト応答がありません。",
    noStderrRecorded: "このスクリプト応答では stderr は記録されませんでした。",
    noStdoutRecorded: "このスクリプト応答では stdout は記録されませんでした。",
    outputWorkLabel: "出力作業",
    outcomeLabel: "結果",
    pendingOutcome: "保留中",
    promptDetailsNotApplicable:
      "このスクリプトベースのディスパッチではプロンプトの詳細は適用されません。",
    requestDetailsTitle: "リクエストの詳細",
    resolvedArgsLabel: "解決済み引数",
    respondedCountLabel: "応答数",
    selectedTraceSuffix: "（選択中）",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `作業項目 ${workItemLabel} を選択`,
    responseDetailsTitle: "応答の詳細",
    scriptAttemptLabel: "スクリプト試行",
    scriptRequestIdLabel: "スクリプトリクエスト ID",
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
    workSelectedActionLabel: "作業を選択中",
  },
  ko: {
    commandLabel: "명령",
    currentDispatchBadge: "현재 디스패치",
    dispatchedCountLabel: "디스패치 수",
    durationLabel: "소요 시간",
    erroredCountLabel: "오류 수",
    exitCodeLabel: "종료 코드",
    failureDetailsTitle: "실패 세부 정보",
    failureMessageLabel: "실패 메시지",
    failureReasonLabel: "실패 원인",
    failureTypeLabel: "실패 유형",
    inferenceAttemptsEmptyEnded:
      "이 디스패치가 끝나기 전까지 추론 시도 세부 정보가 기록되지 않았습니다.",
    inferenceAttemptsEmptyPending:
      "이 디스패치의 추론 시도 세부 정보가 아직 기록되지 않았습니다.",
    inferenceRequestGuidance:
      "추론 요청 세부 정보는 추론 시도 아래에 표시됩니다.",
    inputWorkLabel: "입력 작업",
    noScriptResponseYet: "이 디스패치에는 아직 스크립트 응답이 없습니다.",
    noStderrRecorded: "이 스크립트 응답에는 stderr가 기록되지 않았습니다.",
    noStdoutRecorded: "이 스크립트 응답에는 stdout이 기록되지 않았습니다.",
    outputWorkLabel: "출력 작업",
    outcomeLabel: "결과",
    pendingOutcome: "대기 중",
    promptDetailsNotApplicable:
      "이 스크립트 기반 디스패치에는 프롬프트 세부 정보를 적용할 수 없습니다.",
    requestDetailsTitle: "요청 세부 정보",
    resolvedArgsLabel: "해결된 인수",
    respondedCountLabel: "응답 수",
    selectedTraceSuffix: " (선택됨)",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `작업 항목 ${workItemLabel} 선택`,
    responseDetailsTitle: "응답 세부 정보",
    scriptAttemptLabel: "스크립트 시도",
    scriptRequestIdLabel: "스크립트 요청 ID",
    startedAtLabel: "시작 시각",
    stderrLabel: "표준 오류",
    stdoutLabel: "표준 출력",
    traceDetailsTitle: "추적 세부 정보",
    traceIdsLabel: "추적 ID",
    transitionIdLabel: "전환 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "알 수 없는 디스패치",
    openWorkItemActionLabel: (workItemLabel: string) =>
      `${workItemLabel} 열기`,
    workstationLabel: "워크스테이션",
    workSelectedActionLabel: "작업 선택됨",
  },
  zh: {
    commandLabel: "命令",
    currentDispatchBadge: "当前分派",
    dispatchedCountLabel: "分派次数",
    durationLabel: "耗时",
    erroredCountLabel: "错误次数",
    exitCodeLabel: "退出码",
    failureDetailsTitle: "失败详情",
    failureMessageLabel: "失败消息",
    failureReasonLabel: "失败原因",
    failureTypeLabel: "失败类型",
    inferenceAttemptsEmptyEnded: "该分派结束前没有记录任何推理尝试详情。",
    inferenceAttemptsEmptyPending: "该分派暂时还没有记录推理尝试详情。",
    inferenceRequestGuidance: "推理请求详情显示在推理尝试下方。",
    inputWorkLabel: "输入工作",
    noScriptResponseYet: "这个分派暂时还没有脚本响应。",
    noStderrRecorded: "这个脚本响应没有记录 stderr。",
    noStdoutRecorded: "这个脚本响应没有记录 stdout。",
    outputWorkLabel: "输出工作",
    outcomeLabel: "结果",
    pendingOutcome: "等待中",
    promptDetailsNotApplicable: "这个脚本分派不适用提示词详情。",
    requestDetailsTitle: "请求详情",
    resolvedArgsLabel: "已解析参数",
    respondedCountLabel: "响应次数",
    selectedTraceSuffix: "（已选中）",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `选择工作项 ${workItemLabel}`,
    responseDetailsTitle: "响应详情",
    scriptAttemptLabel: "脚本尝试",
    scriptRequestIdLabel: "脚本请求 ID",
    startedAtLabel: "开始时间",
    stderrLabel: "标准错误",
    stdoutLabel: "标准输出",
    traceDetailsTitle: "追踪详情",
    traceIdsLabel: "追踪 ID",
    transitionIdLabel: "转换 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "未知分派",
    openWorkItemActionLabel: (workItemLabel: string) =>
      `打开 ${workItemLabel}`,
    workstationLabel: "工作站",
    workSelectedActionLabel: "已选中工作项",
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
