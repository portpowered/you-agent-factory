import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CurrentSelectionDispatchHistoryMessages {
  dispatchHistoryCountLabel: (count: number) => string;
  dispatchHistoryEmpty: string;
  dispatchHistoryHeading: string;
  commandLabel: string;
  currentDispatchBadge: string;
  currentSelectionUnavailableValue: string;
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
  modelLabel: string;
  outputWorkLabel: string;
  outcomeLabel: string;
  pendingOutcome: string;
  providerLabel: string;
  recordedOutcome: string;
  promptDetailsNotApplicable: string;
  requestDetailsTitle: string;
  requestAttemptTitle: (attemptNumber: number | undefined) => string;
  requestAttemptFallbackId: string;
  resolvedArgsLabel: string;
  respondedCountLabel: string;
  responseAttemptTitle: (attemptNumber: number | undefined) => string;
  responseAttemptFallbackId: string;
  relationshipChildLabel: string;
  relationshipDependsOnLabel: string;
  relationshipLaneAriaLabel: (label: string) => string;
  relationshipParentLabel: string;
  relationshipRelatedLabel: string;
  relationshipRequiredByLabel: string;
  relatedWorkSelectLabel: (workItemLabel: string) => string;
  relationshipStateLabel: (label: string, requiredState: string) => string;
  selectedTraceSuffix: string;
  selectWorkItemAccessibleLabel: (workItemLabel: string) => string;
  responseDetailsTitle: string;
  runtimeLabelsLabel: string;
  scriptAttemptsEmpty: string;
  scriptAttemptsHeading: string;
  scriptAttemptLabel: string;
  scriptRequestIdLabel: string;
  startedAtLabel: string;
  stderrLabel: string;
  stdoutLabel: string;
  workingDirectoryLabel: string;
  worktreeLabel: string;
  inferenceAttemptsHeading: string;
  traceDetailsTitle: string;
  traceIdsLabel: string;
  transitionIdLabel: string;
  unknownDispatchId: string;
  unknownDispatchTitle: string;
  openWorkItemActionLabel: (workItemLabel: string) => string;
  workIdLabel: string;
  workTypeLabel: string;
  selectedWorkHeading: string;
  workRelationshipsEmpty: string;
  workRelationshipsHeading: string;
  workstationDispatchesLabel: string;
  workstationLabel: string;
  workstationUnavailableValue: string;
  workSelectedActionLabel: string;
}

const singularPlural = (count: number, singular: string, plural: string) =>
  `${count} ${count === 1 ? singular : plural}`;

const currentSelectionDispatchHistoryMessagesByLocale = {
  en: {
    dispatchHistoryCountLabel: (count: number) =>
      singularPlural(count, "dispatch", "dispatches"),
    dispatchHistoryEmpty:
      "No workstation dispatch has been recorded yet for this work item.",
    dispatchHistoryHeading: "Workstation dispatches",
    commandLabel: "Command",
    currentDispatchBadge: "Current dispatch",
    currentSelectionUnavailableValue: "Unavailable",
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
    modelLabel: "Model",
    outputWorkLabel: "Output work",
    outcomeLabel: "Outcome",
    pendingOutcome: "PENDING",
    providerLabel: "Provider",
    recordedOutcome: "RECORDED",
    promptDetailsNotApplicable:
      "Prompt details are not applicable to this script-backed dispatch.",
    requestDetailsTitle: "Request details",
    requestAttemptTitle: (attemptNumber: number | undefined) =>
      `Request attempt ${attemptNumber ?? "pending"}`,
    requestAttemptFallbackId: "script-request",
    relationshipChildLabel: "Child",
    relationshipDependsOnLabel: "Depends on",
    relationshipLaneAriaLabel: (label: string) => `${label} relationships`,
    relationshipParentLabel: "Parent",
    relationshipRelatedLabel: "Related",
    relationshipRequiredByLabel: "Required by",
    relatedWorkSelectLabel: (workItemLabel: string) =>
      `Select related work item ${workItemLabel}`,
    relationshipStateLabel: (label: string, requiredState: string) =>
      `${label} (${requiredState})`,
    resolvedArgsLabel: "Resolved args",
    respondedCountLabel: "respondedCount",
    responseAttemptTitle: (attemptNumber: number | undefined) =>
      `Response attempt ${attemptNumber ?? "completed"}`,
    responseAttemptFallbackId: "script-response",
    selectedTraceSuffix: " (selected)",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `Select work item ${workItemLabel}`,
    responseDetailsTitle: "Response details",
    runtimeLabelsLabel: "Runtime labels",
    scriptAttemptsEmpty: "No script response attempt has been recorded yet.",
    scriptAttemptsHeading: "Script attempts",
    scriptAttemptLabel: "Script attempt",
    scriptRequestIdLabel: "Script request ID",
    startedAtLabel: "Started at",
    stderrLabel: "Stderr",
    stdoutLabel: "Stdout",
    workingDirectoryLabel: "Working directory",
    worktreeLabel: "Worktree",
    inferenceAttemptsHeading: "Inference attempts",
    traceDetailsTitle: "Trace details",
    traceIdsLabel: "Trace IDs",
    transitionIdLabel: "Transition ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "Unknown dispatch",
    openWorkItemActionLabel: (workItemLabel: string) => `Open ${workItemLabel}`,
    workIdLabel: "Work ID",
    workTypeLabel: "Work type",
    selectedWorkHeading: "Selected work",
    workRelationshipsEmpty:
      "No parent, child, or dependency relationships are available for this work item.",
    workRelationshipsHeading: "Work relationships",
    workstationDispatchesLabel: "Workstation dispatches",
    workstationLabel: "Workstation",
    workstationUnavailableValue: "Unavailable",
    workSelectedActionLabel: "Work selected",
  },
  ja: {
    dispatchHistoryCountLabel: (count: number) => `${count} 件のディスパッチ`,
    dispatchHistoryEmpty:
      "この作業項目ではまだワークステーションのディスパッチが記録されていません。",
    dispatchHistoryHeading: "ワークステーションのディスパッチ",
    commandLabel: "コマンド",
    currentDispatchBadge: "現在のディスパッチ",
    currentSelectionUnavailableValue: "利用不可",
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
    modelLabel: "モデル",
    outputWorkLabel: "出力作業",
    outcomeLabel: "結果",
    pendingOutcome: "保留中",
    providerLabel: "プロバイダー",
    recordedOutcome: "記録済み",
    promptDetailsNotApplicable:
      "このスクリプトベースのディスパッチではプロンプトの詳細は適用されません。",
    requestDetailsTitle: "リクエストの詳細",
    requestAttemptTitle: (attemptNumber: number | undefined) =>
      `リクエスト試行 ${attemptNumber ?? "保留中"}`,
    requestAttemptFallbackId: "script-request",
    relationshipChildLabel: "子作業",
    relationshipDependsOnLabel: "依存先",
    relationshipLaneAriaLabel: (label: string) => `${label} の関係`,
    relationshipParentLabel: "親作業",
    relationshipRelatedLabel: "関連",
    relationshipRequiredByLabel: "依存元",
    relatedWorkSelectLabel: (workItemLabel: string) =>
      `関連する作業項目 ${workItemLabel} を選択`,
    relationshipStateLabel: (label: string, requiredState: string) =>
      `${label}（${requiredState}）`,
    resolvedArgsLabel: "解決済み引数",
    respondedCountLabel: "応答数",
    responseAttemptTitle: (attemptNumber: number | undefined) =>
      `応答試行 ${attemptNumber ?? "完了"}`,
    responseAttemptFallbackId: "script-response",
    selectedTraceSuffix: "（選択中）",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `作業項目 ${workItemLabel} を選択`,
    responseDetailsTitle: "応答の詳細",
    runtimeLabelsLabel: "ランタイムラベル",
    scriptAttemptsEmpty:
      "このディスパッチではまだスクリプト応答の試行が記録されていません。",
    scriptAttemptsHeading: "スクリプト試行",
    scriptAttemptLabel: "スクリプト試行",
    scriptRequestIdLabel: "スクリプトリクエスト ID",
    startedAtLabel: "開始時刻",
    stderrLabel: "標準エラー",
    stdoutLabel: "標準出力",
    workingDirectoryLabel: "作業ディレクトリ",
    worktreeLabel: "ワークツリー",
    inferenceAttemptsHeading: "推論試行",
    traceDetailsTitle: "トレースの詳細",
    traceIdsLabel: "トレース ID",
    transitionIdLabel: "遷移 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "不明なディスパッチ",
    openWorkItemActionLabel: (workItemLabel: string) =>
      `${workItemLabel} を開く`,
    workIdLabel: "作業 ID",
    workTypeLabel: "作業タイプ",
    selectedWorkHeading: "選択中の作業",
    workRelationshipsEmpty:
      "この作業項目では親子関係や依存関係を利用できません。",
    workRelationshipsHeading: "作業の関係",
    workstationDispatchesLabel: "ワークステーションのディスパッチ",
    workstationLabel: "ワークステーション",
    workstationUnavailableValue: "利用不可",
    workSelectedActionLabel: "作業を選択中",
  },
  ko: {
    dispatchHistoryCountLabel: (count: number) => `${count}개 디스패치`,
    dispatchHistoryEmpty:
      "이 작업 항목에는 아직 워크스테이션 디스패치가 기록되지 않았습니다.",
    dispatchHistoryHeading: "워크스테이션 디스패치",
    commandLabel: "명령",
    currentDispatchBadge: "현재 디스패치",
    currentSelectionUnavailableValue: "사용할 수 없음",
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
    modelLabel: "모델",
    outputWorkLabel: "출력 작업",
    outcomeLabel: "결과",
    pendingOutcome: "대기 중",
    providerLabel: "공급자",
    recordedOutcome: "기록됨",
    promptDetailsNotApplicable:
      "이 스크립트 기반 디스패치에는 프롬프트 세부 정보를 적용할 수 없습니다.",
    requestDetailsTitle: "요청 세부 정보",
    requestAttemptTitle: (attemptNumber: number | undefined) =>
      `요청 시도 ${attemptNumber ?? "대기 중"}`,
    requestAttemptFallbackId: "script-request",
    relationshipChildLabel: "하위 작업",
    relationshipDependsOnLabel: "의존 대상",
    relationshipLaneAriaLabel: (label: string) => `${label} 관계`,
    relationshipParentLabel: "상위 작업",
    relationshipRelatedLabel: "관련됨",
    relationshipRequiredByLabel: "의존하는 작업",
    relatedWorkSelectLabel: (workItemLabel: string) =>
      `관련 작업 항목 ${workItemLabel} 선택`,
    relationshipStateLabel: (label: string, requiredState: string) =>
      `${label} (${requiredState})`,
    resolvedArgsLabel: "해결된 인수",
    respondedCountLabel: "응답 수",
    responseAttemptTitle: (attemptNumber: number | undefined) =>
      `응답 시도 ${attemptNumber ?? "완료"}`,
    responseAttemptFallbackId: "script-response",
    selectedTraceSuffix: " (선택됨)",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `작업 항목 ${workItemLabel} 선택`,
    responseDetailsTitle: "응답 세부 정보",
    runtimeLabelsLabel: "런타임 레이블",
    scriptAttemptsEmpty: "이 디스패치에는 아직 스크립트 응답 시도가 기록되지 않았습니다.",
    scriptAttemptsHeading: "스크립트 시도",
    scriptAttemptLabel: "스크립트 시도",
    scriptRequestIdLabel: "스크립트 요청 ID",
    startedAtLabel: "시작 시각",
    stderrLabel: "표준 오류",
    stdoutLabel: "표준 출력",
    workingDirectoryLabel: "작업 디렉터리",
    worktreeLabel: "워크트리",
    inferenceAttemptsHeading: "추론 시도",
    traceDetailsTitle: "추적 세부 정보",
    traceIdsLabel: "추적 ID",
    transitionIdLabel: "전환 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "알 수 없는 디스패치",
    openWorkItemActionLabel: (workItemLabel: string) => `${workItemLabel} 열기`,
    workIdLabel: "작업 ID",
    workTypeLabel: "작업 유형",
    selectedWorkHeading: "선택한 작업",
    workRelationshipsEmpty:
      "이 작업 항목에는 상하위 또는 의존 관계가 없습니다.",
    workRelationshipsHeading: "작업 관계",
    workstationDispatchesLabel: "워크스테이션 디스패치",
    workstationLabel: "워크스테이션",
    workstationUnavailableValue: "사용할 수 없음",
    workSelectedActionLabel: "작업 선택됨",
  },
  "zh-CN": {
    dispatchHistoryCountLabel: (count: number) => `${count} 次分派`,
    dispatchHistoryEmpty: "这个工作项暂时还没有记录任何工作站分派。",
    dispatchHistoryHeading: "工作站分派",
    commandLabel: "命令",
    currentDispatchBadge: "当前分派",
    currentSelectionUnavailableValue: "不可用",
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
    modelLabel: "模型",
    outputWorkLabel: "输出工作",
    outcomeLabel: "结果",
    pendingOutcome: "等待中",
    providerLabel: "提供方",
    recordedOutcome: "已记录",
    promptDetailsNotApplicable: "这个脚本分派不适用提示词详情。",
    requestDetailsTitle: "请求详情",
    requestAttemptTitle: (attemptNumber: number | undefined) =>
      `请求尝试 ${attemptNumber ?? "等待中"}`,
    requestAttemptFallbackId: "script-request",
    relationshipChildLabel: "子工作",
    relationshipDependsOnLabel: "依赖项",
    relationshipLaneAriaLabel: (label: string) => `${label}关系`,
    relationshipParentLabel: "父工作",
    relationshipRelatedLabel: "相关",
    relationshipRequiredByLabel: "依赖于此",
    relatedWorkSelectLabel: (workItemLabel: string) =>
      `选择相关工作项 ${workItemLabel}`,
    relationshipStateLabel: (label: string, requiredState: string) =>
      `${label}（${requiredState}）`,
    resolvedArgsLabel: "已解析参数",
    respondedCountLabel: "响应次数",
    responseAttemptTitle: (attemptNumber: number | undefined) =>
      `响应尝试 ${attemptNumber ?? "已完成"}`,
    responseAttemptFallbackId: "script-response",
    selectedTraceSuffix: "（已选中）",
    selectWorkItemAccessibleLabel: (workItemLabel: string) =>
      `选择工作项 ${workItemLabel}`,
    responseDetailsTitle: "响应详情",
    runtimeLabelsLabel: "运行时标签",
    scriptAttemptsEmpty: "这个分派暂时还没有记录脚本响应尝试。",
    scriptAttemptsHeading: "脚本尝试",
    scriptAttemptLabel: "脚本尝试",
    scriptRequestIdLabel: "脚本请求 ID",
    startedAtLabel: "开始时间",
    stderrLabel: "标准错误",
    stdoutLabel: "标准输出",
    workingDirectoryLabel: "工作目录",
    worktreeLabel: "工作树",
    inferenceAttemptsHeading: "推理尝试",
    traceDetailsTitle: "追踪详情",
    traceIdsLabel: "追踪 ID",
    transitionIdLabel: "转换 ID",
    unknownDispatchId: "unknown-dispatch",
    unknownDispatchTitle: "未知分派",
    openWorkItemActionLabel: (workItemLabel: string) => `打开 ${workItemLabel}`,
    workIdLabel: "工作 ID",
    workTypeLabel: "工作类型",
    selectedWorkHeading: "已选工作",
    workRelationshipsEmpty: "这个工作项暂时没有可用的父子或依赖关系。",
    workRelationshipsHeading: "工作关系",
    workstationDispatchesLabel: "工作站分派",
    workstationLabel: "工作站",
    workstationUnavailableValue: "不可用",
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
