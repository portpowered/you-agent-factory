import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CurrentSelectionShellMessages {
  completedStatus: string;
  completedTerminalWorkSummary: string;
  dispatchIdLabel: string;
  dispatchIdUnavailable: string;
  elapsedLabel: string;
  elapsedUnavailable: string;
  emptyStateGuidance: string;
  executionDetailsHeading: string;
  executionDetailsRegionLabel: string;
  failedStatus: string;
  failureDetailsUnavailable: string;
  failureMessageLabel: string;
  failureMessageUnavailable: string;
  failureReasonLabel: string;
  failureReasonUnavailable: string;
  inferenceAttemptsEmptyState: string;
  inferenceAttemptsHeading: string;
  inferenceAttemptsRegionLabel: string;
  openTraceAction: string;
  selectedTraceSuffix: string;
  sourceLabel: string;
  sourceSummary: string;
  statusLabel: string;
  title: string;
  traceGuidance: string;
  traceIdsLabel: string;
  traceUnavailable: string;
  workstationLabel: string;
  workstationRequestGuidance: string;
  workstationRequestHeading: string;
  workstationRequestRegionLabel: string;
  workstationUnavailable: string;
  redoAccessibleLabel: string;
  redoLabel: string;
  undoAccessibleLabel: string;
  undoLabel: string;
}

const currentSelectionShellMessagesByLocale = {
  en: {
    completedStatus: "Completed",
    completedTerminalWorkSummary:
      "Completed terminal work is retained in the session summary.",
    dispatchIdLabel: "Dispatch ID",
    dispatchIdUnavailable:
      "Dispatch ID is not available for this selected run.",
    elapsedLabel: "Elapsed",
    elapsedUnavailable: "Elapsed time is not available for this selected run.",
    emptyStateGuidance:
      "Select a workstation, work item, or state node to inspect live details.",
    executionDetailsHeading: "Execution details",
    executionDetailsRegionLabel: "Execution details",
    failedStatus: "Failed",
    failureDetailsUnavailable:
      "Failure details are unavailable for this failed work item.",
    failureMessageLabel: "Failure message",
    failureMessageUnavailable: "Failure message unavailable",
    failureReasonLabel: "Failure reason",
    failureReasonUnavailable: "Failure reason unavailable",
    inferenceAttemptsEmptyState:
      "No inference events are available for this selected work item.",
    inferenceAttemptsHeading: "Inference attempts",
    inferenceAttemptsRegionLabel: "Inference attempts",
    openTraceAction: "Open trace",
    selectedTraceSuffix: " (selected)",
    sourceLabel: "Source",
    sourceSummary: "Current workstation run summary",
    statusLabel: "Status",
    title: "Current selection",
    traceGuidance:
      "Open the trace to review dispatches, retries, and workstation output for this work item.",
    traceIdsLabel: "Trace IDs",
    traceUnavailable: "Trace details are not available for this selected run.",
    undoAccessibleLabel: "Undo selection",
    undoLabel: "Undo",
    workstationLabel: "Workstation",
    workstationRequestGuidance:
      "Prompt, provider-session, and response-body details are shown under Inference attempts.",
    workstationRequestHeading: "Workstation request",
    workstationRequestRegionLabel: "Workstation request",
    workstationUnavailable:
      "Workstation details are not available for this selected run.",
    redoAccessibleLabel: "Redo selection",
    redoLabel: "Redo",
  },
  ja: {
    completedStatus: "完了",
    completedTerminalWorkSummary:
      "完了した端末作業はセッション概要に保持されます。",
    dispatchIdLabel: "ディスパッチ ID",
    dispatchIdUnavailable:
      "この選択中の実行ではディスパッチ ID を利用できません。",
    elapsedLabel: "経過時間",
    elapsedUnavailable: "この選択中の実行では経過時間を利用できません。",
    emptyStateGuidance:
      "ライブの詳細を確認するには、ワークステーション、作業項目、または状態ノードを選択してください。",
    executionDetailsHeading: "実行の詳細",
    executionDetailsRegionLabel: "実行の詳細",
    failedStatus: "失敗",
    failureDetailsUnavailable:
      "この失敗した作業項目では失敗の詳細を利用できません。",
    failureMessageLabel: "失敗メッセージ",
    failureMessageUnavailable: "失敗メッセージを利用できません",
    failureReasonLabel: "失敗理由",
    failureReasonUnavailable: "失敗理由を利用できません",
    inferenceAttemptsEmptyState:
      "この選択中の作業項目では推論イベントを利用できません。",
    inferenceAttemptsHeading: "推論試行",
    inferenceAttemptsRegionLabel: "推論試行",
    openTraceAction: "トレースを開く",
    selectedTraceSuffix: "（選択中）",
    redoAccessibleLabel: "選択をやり直す",
    redoLabel: "やり直す",
    sourceLabel: "ソース",
    sourceSummary: "現在のワークステーション実行概要",
    statusLabel: "ステータス",
    title: "現在の選択",
    traceGuidance:
      "この作業項目のディスパッチ、再試行、ワークステーション出力を確認するにはトレースを開いてください。",
    traceIdsLabel: "トレース ID",
    traceUnavailable: "この選択中の実行ではトレースの詳細を利用できません。",
    undoAccessibleLabel: "選択を元に戻す",
    undoLabel: "元に戻す",
    workstationLabel: "ワークステーション",
    workstationRequestGuidance:
      "プロンプト、provider-session、response-body の詳細は推論試行の下に表示されます。",
    workstationRequestHeading: "ワークステーション要求",
    workstationRequestRegionLabel: "ワークステーション要求",
    workstationUnavailable:
      "この選択中の実行ではワークステーションの詳細を利用できません。",
  },
  ko: {
    completedStatus: "완료됨",
    completedTerminalWorkSummary:
      "완료된 터미널 작업은 세션 요약에 유지됩니다.",
    dispatchIdLabel: "디스패치 ID",
    dispatchIdUnavailable:
      "선택한 실행에서는 디스패치 ID를 사용할 수 없습니다.",
    elapsedLabel: "경과 시간",
    elapsedUnavailable: "선택한 실행에서는 경과 시간을 사용할 수 없습니다.",
    emptyStateGuidance:
      "실시간 세부 정보를 확인하려면 워크스테이션, 작업 항목 또는 상태 노드를 선택하세요.",
    executionDetailsHeading: "실행 세부 정보",
    executionDetailsRegionLabel: "실행 세부 정보",
    failedStatus: "실패",
    failureDetailsUnavailable:
      "이 실패한 작업 항목에서는 실패 세부 정보를 사용할 수 없습니다.",
    failureMessageLabel: "실패 메시지",
    failureMessageUnavailable: "실패 메시지를 사용할 수 없음",
    failureReasonLabel: "실패 원인",
    failureReasonUnavailable: "실패 원인을 사용할 수 없음",
    inferenceAttemptsEmptyState:
      "선택한 작업 항목에는 사용할 수 있는 추론 이벤트가 없습니다.",
    inferenceAttemptsHeading: "추론 시도",
    inferenceAttemptsRegionLabel: "추론 시도",
    openTraceAction: "추적 열기",
    selectedTraceSuffix: " (선택됨)",
    redoAccessibleLabel: "선택 다시 실행",
    redoLabel: "다시 실행",
    sourceLabel: "원본",
    sourceSummary: "현재 워크스테이션 실행 요약",
    statusLabel: "상태",
    title: "현재 선택",
    traceGuidance:
      "이 작업 항목의 디스패치, 재시도, 워크스테이션 출력을 검토하려면 추적을 여세요.",
    traceIdsLabel: "추적 ID",
    traceUnavailable: "선택한 실행에서는 추적 세부 정보를 사용할 수 없습니다.",
    undoAccessibleLabel: "선택 실행 취소",
    undoLabel: "실행 취소",
    workstationLabel: "워크스테이션",
    workstationRequestGuidance:
      "프롬프트, provider-session, response-body 세부 정보는 추론 시도 아래에 표시됩니다.",
    workstationRequestHeading: "워크스테이션 요청",
    workstationRequestRegionLabel: "워크스테이션 요청",
    workstationUnavailable:
      "선택한 실행에서는 워크스테이션 세부 정보를 사용할 수 없습니다.",
  },
  zh: {
    completedStatus: "已完成",
    completedTerminalWorkSummary: "已完成的终端工作会保留在会话摘要中。",
    dispatchIdLabel: "分派 ID",
    dispatchIdUnavailable: "当前所选运行暂时没有分派 ID。",
    elapsedLabel: "耗时",
    elapsedUnavailable: "当前所选运行暂时没有耗时信息。",
    emptyStateGuidance: "选择工作站、工作项或状态节点以查看实时详细信息。",
    executionDetailsHeading: "执行详情",
    executionDetailsRegionLabel: "执行详情",
    failedStatus: "失败",
    failureDetailsUnavailable: "这个失败的工作项暂时没有失败详情。",
    failureMessageLabel: "失败消息",
    failureMessageUnavailable: "暂无失败消息",
    failureReasonLabel: "失败原因",
    failureReasonUnavailable: "暂无失败原因",
    inferenceAttemptsEmptyState: "当前所选工作项暂时没有推理事件。",
    inferenceAttemptsHeading: "推理尝试",
    inferenceAttemptsRegionLabel: "推理尝试",
    openTraceAction: "打开追踪",
    selectedTraceSuffix: "（已选中）",
    redoAccessibleLabel: "重做所选内容",
    redoLabel: "重做",
    sourceLabel: "来源",
    sourceSummary: "当前工作站运行摘要",
    statusLabel: "状态",
    title: "当前选择",
    traceGuidance: "打开追踪以查看该工作项的分派、重试和工作站输出。",
    traceIdsLabel: "追踪 ID",
    traceUnavailable: "当前所选运行暂时没有追踪详情。",
    undoAccessibleLabel: "撤销所选内容",
    undoLabel: "撤销",
    workstationLabel: "工作站",
    workstationRequestGuidance:
      "提示词、provider-session 和 response-body 详情显示在推理尝试下方。",
    workstationRequestHeading: "工作站请求",
    workstationRequestRegionLabel: "工作站请求",
    workstationUnavailable: "当前所选运行暂时没有工作站详情。",
  },
} satisfies LocalizedMessages<CurrentSelectionShellMessages>;

export function getCurrentSelectionShellMessages(
  locale?: string | null,
): CurrentSelectionShellMessages {
  return resolveLocalizedMessages(
    currentSelectionShellMessagesByLocale,
    locale,
  );
}

export { currentSelectionShellMessagesByLocale };
