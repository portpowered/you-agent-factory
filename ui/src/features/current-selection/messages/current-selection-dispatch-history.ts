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
  respondedCountLabel: string;
  responseDetailsTitle: string;
  scriptAttemptLabel: string;
  scriptRequestIdLabel: string;
  startedAtLabel: string;
  stderrLabel: string;
  stdoutLabel: string;
  traceDetailsTitle: string;
  transitionIdLabel: string;
  unknownDispatchId: string;
  unknownDispatchTitle: string;
  workstationLabel: string;
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
    respondedCountLabel: "respondedCount",
    responseDetailsTitle: "Response details",
    scriptAttemptLabel: "Script attempt",
    scriptRequestIdLabel: "Script request ID",
    startedAtLabel: "Started at",
    stderrLabel: "Stderr",
    stdoutLabel: "Stdout",
    traceDetailsTitle: "Trace details",
    transitionIdLabel: "Transition ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "Unknown dispatch",
    workstationLabel: "Workstation",
  },
  ja: {
    commandLabel: "コマンド",
    currentDispatchBadge: "現在のディスパッチ",
    dispatchedCountLabel: "dispatchedCount",
    durationLabel: "所要時間",
    erroredCountLabel: "erroredCount",
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
    pendingOutcome: "PENDING",
    promptDetailsNotApplicable:
      "このスクリプトベースのディスパッチではプロンプトの詳細は適用されません。",
    requestDetailsTitle: "リクエストの詳細",
    respondedCountLabel: "respondedCount",
    responseDetailsTitle: "応答の詳細",
    scriptAttemptLabel: "スクリプト試行",
    scriptRequestIdLabel: "スクリプトリクエスト ID",
    startedAtLabel: "開始時刻",
    stderrLabel: "Stderr",
    stdoutLabel: "Stdout",
    traceDetailsTitle: "トレースの詳細",
    transitionIdLabel: "Transition ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "不明なディスパッチ",
    workstationLabel: "ワークステーション",
  },
  ko: {
    commandLabel: "명령",
    currentDispatchBadge: "현재 디스패치",
    dispatchedCountLabel: "dispatchedCount",
    durationLabel: "소요 시간",
    erroredCountLabel: "erroredCount",
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
    pendingOutcome: "PENDING",
    promptDetailsNotApplicable:
      "이 스크립트 기반 디스패치에는 프롬프트 세부 정보를 적용할 수 없습니다.",
    requestDetailsTitle: "요청 세부 정보",
    respondedCountLabel: "respondedCount",
    responseDetailsTitle: "응답 세부 정보",
    scriptAttemptLabel: "스크립트 시도",
    scriptRequestIdLabel: "스크립트 요청 ID",
    startedAtLabel: "시작 시각",
    stderrLabel: "Stderr",
    stdoutLabel: "Stdout",
    traceDetailsTitle: "추적 세부 정보",
    transitionIdLabel: "Transition ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "알 수 없는 디스패치",
    workstationLabel: "워크스테이션",
  },
  zh: {
    commandLabel: "命令",
    currentDispatchBadge: "当前分派",
    dispatchedCountLabel: "dispatchedCount",
    durationLabel: "耗时",
    erroredCountLabel: "erroredCount",
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
    pendingOutcome: "PENDING",
    promptDetailsNotApplicable: "这个脚本分派不适用提示词详情。",
    requestDetailsTitle: "请求详情",
    respondedCountLabel: "respondedCount",
    responseDetailsTitle: "响应详情",
    scriptAttemptLabel: "脚本尝试",
    scriptRequestIdLabel: "脚本请求 ID",
    startedAtLabel: "开始时间",
    stderrLabel: "Stderr",
    stdoutLabel: "Stdout",
    traceDetailsTitle: "追踪详情",
    transitionIdLabel: "Transition ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "未知分派",
    workstationLabel: "工作站",
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
