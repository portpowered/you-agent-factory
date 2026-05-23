// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: feature-local locale catalogs keep required language coverage in one typed asset set.
import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CurrentSelectionDetailMessages {
  attemptAriaLabel: (attemptNumber: number) => string;
  attemptTitle: (attemptNumber: number) => string;
  collapseAttemptAction: (attemptNumber: number) => string;
  collapseRequestBodyAction: string;
  collapseResponseBodyAction: string;
  awaitingProviderResponse: string;
  commandLabel: string;
  commandUnavailable: string;
  consumedPayloadEmpty: string;
  consumedPayloadError: string;
  consumedPayloadHeading: string;
  consumedPayloadLoading: string;
  consumedPayloadUnavailable: string;
  countLabel: string;
  consumedWorkItemsLabel: string;
  currentWorkHeading: string;
  dispatchIdLabel: string;
  durationLabel: string;
  durationUnavailable: string;
  elapsedTimeLabel: string;
  errorClassLabel: string;
  errorDetailsTitle: string;
  exitCodeLabel: string;
  exitCodeUnavailable: string;
  failureMessageLabel: string;
  failureMessageUnavailable: string;
  failureReasonLabel: string;
  failureReasonUnavailable: string;
  failureTypeLabel: string;
  failureTypeUnavailable: string;
  inferenceRequestDetailsCopy: string;
  inferenceRequestIdLabel: string;
  inferenceResponseDetailsCopy: string;
  metadataEmpty: string;
  modelLabel: string;
  outcomeLabel: string;
  outcomeUnavailable: string;
  pendingOutcome: string;
  providerLabel: string;
  providerResponseUnavailable: string;
  providerSessionLabel: string;
  runnerLabel: string;
  runnerCapabilityImageInputLabel: string;
  runnerCapabilitySessionResumeLabel: string;
  runnerCapabilityStructuredOutputLabel: string;
  runnerCapabilitySupportHeading: string;
  runnerCapabilitySupportedLabel: string;
  runnerCapabilityUnsupportedLabel: string;
  runnerCapabilityWorkingDirectoryLabel: string;
  runnerCapabilityWorktreeLabel: string;
  runnerSelectionSourceLabel: string;
  collapseAction: string;
  expandAction: string;
  expandAttemptAction: (attemptNumber: number) => string;
  expandRequestBodyAction: string;
  expandResponseBodyAction: string;
  noCurrentWorkInPlace: string;
  noWorkRecordedAtSelectedTick: string;
  requestBodyLabel: string;
  requestDetailsTitle: string;
  requestIdLabel: string;
  requestIdUnavailable: string;
  requestMetadataTitle: string;
  requestTimeLabel: string;
  resolvedArgsLabel: string;
  responseBodyLabel: string;
  responseDetailsTitle: string;
  responseMetadataTitle: string;
  responseMetadataUnavailableErrored: string;
  responseMetadataUnavailableScript: string;
  responseTimeLabel: string;
  timestampUnavailable: string;
  selectedTickWorkUnavailable: string;
  scriptArgumentsUnavailable: string;
  scriptAttemptLabel: string;
  scriptAttemptUnavailable: string;
  scriptRequestIdLabel: string;
  scriptRequestUnavailable: string;
  scriptResponseUnavailableErrored: string;
  scriptResponseUnavailablePending: string;
  scriptResponseUnavailableSummary: string;
  selectWorkItemLabel: (workItemLabel: string) => string;
  startedAtLabel: string;
  stderrEmpty: string;
  stderrLabel: string;
  stdoutEmpty: string;
  stdoutLabel: string;
  stateLabel: string;
  stateNodeIdLabel: string;
  totalDurationLabel: string;
  totalDurationUnavailable: string;
  terminalOutcomeLabel: (outcome: string) => string;
  terminalRequestContext: (params: {
    outcome: string;
    providerSession?: string;
    workstation: string;
  }) => string;
  traceIdLabel: string;
  traceIdsLabel: string;
  traceUnavailable: string;
  transitionIdLabel: string;
  workIdLabel: string;
  worktreeLabel: string;
  workstationLabel: string;
  workstationUnavailable: string;
  workTypeLabel: string;
  workTypeUnavailable: string;
  workingDirectoryLabel: string;
}

const stateNodeDetailFallbackMessages = {
  countLabel: "Count",
  currentWorkHeading: "Current work",
  noCurrentWorkInPlace: "No current work is occupying this place.",
  noWorkRecordedAtSelectedTick:
    "No work is recorded for this place at the selected tick.",
  selectedTickWorkUnavailable:
    "Represented work is unavailable for this place at the selected tick.",
  startedAtLabel: "Started at",
  stateLabel: "State",
  stateNodeIdLabel: "State node ID",
  traceIdLabel: "Trace ID",
  workIdLabel: "Work ID",
  workTypeLabel: "Work type",
  workTypeUnavailable: "Unknown",
} satisfies Pick<
  CurrentSelectionDetailMessages,
  | "countLabel"
  | "currentWorkHeading"
  | "noCurrentWorkInPlace"
  | "noWorkRecordedAtSelectedTick"
  | "selectedTickWorkUnavailable"
  | "startedAtLabel"
  | "stateLabel"
  | "stateNodeIdLabel"
  | "traceIdLabel"
  | "workIdLabel"
  | "workTypeLabel"
  | "workTypeUnavailable"
>;

const currentSelectionDetailMessagesByLocale = {
  en: {
    ...stateNodeDetailFallbackMessages,
    attemptAriaLabel: (attemptNumber: number) =>
      `Inference attempt ${attemptNumber}`,
    attemptTitle: (attemptNumber: number) => `Attempt ${attemptNumber}`,
    collapseAttemptAction: (attemptNumber: number) =>
      `Collapse attempt ${attemptNumber}`,
    collapseRequestBodyAction: "Collapse request body",
    collapseResponseBodyAction: "Collapse response body",
    awaitingProviderResponse: "Awaiting provider response.",
    commandLabel: "Command",
    commandUnavailable:
      "Script command details are not available for this workstation request.",
    consumedPayloadEmpty:
      "No consumed payload content was recorded for this work item.",
    consumedPayloadError:
      "Consumed payload details could not be loaded for this work item.",
    consumedPayloadHeading: "Consumed payload",
    consumedPayloadLoading:
      "Consumed payload details are still loading for this work item.",
    consumedPayloadUnavailable:
      "Consumed payload details are unavailable for this work item.",
    consumedWorkItemsLabel: "Consumed work items",
    dispatchIdLabel: "Dispatch ID",
    durationLabel: "Duration",
    durationUnavailable:
      "Duration details are not available for this script response yet.",
    elapsedTimeLabel: "Elapsed time",
    errorClassLabel: "Error class",
    errorDetailsTitle: "Error details",
    exitCodeLabel: "Exit code",
    exitCodeUnavailable: "Exit code is not available for this script response.",
    failureMessageLabel: "Failure message",
    failureMessageUnavailable:
      "Failure message is not available for this request.",
    failureReasonLabel: "Failure reason",
    failureReasonUnavailable:
      "Failure reason is not available for this request.",
    failureTypeLabel: "Failure type",
    failureTypeUnavailable:
      "Failure type is not available for this script response.",
    inferenceRequestDetailsCopy:
      "Prompt, request payload, working-directory, and worktree details are shown under Inference attempts when available.",
    inferenceRequestIdLabel: "Inference request ID",
    inferenceResponseDetailsCopy:
      "Response, provider-session, and inference metadata details are shown under Inference attempts when available.",
    metadataEmpty:
      "Request metadata is not available for this workstation request.",
    modelLabel: "Model",
    outcomeLabel: "Outcome",
    outcomeUnavailable: "Outcome details are not available yet.",
    pendingOutcome: "PENDING",
    providerLabel: "Provider",
    providerResponseUnavailable:
      "Provider response text is not available for this inference attempt.",
    providerSessionLabel: "Provider session",
    runnerLabel: "Runner",
    runnerCapabilityImageInputLabel: "Image input",
    runnerCapabilitySessionResumeLabel: "Session resume",
    runnerCapabilityStructuredOutputLabel: "Structured output",
    runnerCapabilitySupportHeading: "Runner capability support",
    runnerCapabilitySupportedLabel: "Supported",
    runnerCapabilityUnsupportedLabel: "Unsupported",
    runnerCapabilityWorkingDirectoryLabel: "Working directory",
    runnerCapabilityWorktreeLabel: "Worktree selection",
    runnerSelectionSourceLabel: "Runner source",
    collapseAction: "Collapse",
    expandAction: "Expand",
    expandAttemptAction: (attemptNumber: number) =>
      `Expand attempt ${attemptNumber}`,
    expandRequestBodyAction: "Expand request body",
    expandResponseBodyAction: "Expand response body",
    requestBodyLabel: "Request body",
    requestDetailsTitle: "Request details",
    requestIdLabel: "Request ID",
    requestIdUnavailable:
      "Request ID is not available for this workstation request.",
    requestMetadataTitle: "Request metadata",
    requestTimeLabel: "Request time",
    resolvedArgsLabel: "Resolved args",
    responseBodyLabel: "Response body",
    responseDetailsTitle: "Response details",
    responseMetadataTitle: "Response metadata",
    responseMetadataUnavailableErrored:
      "Response metadata is unavailable because this workstation request ended with an error.",
    responseMetadataUnavailableScript:
      "Response metadata is not available for this script-backed workstation request.",
    responseTimeLabel: "Response time",
    timestampUnavailable: "Unavailable",
    scriptArgumentsUnavailable:
      "Script arguments are not available for this workstation request.",
    scriptAttemptLabel: "Script attempt",
    scriptAttemptUnavailable: "Script attempt is not available yet.",
    scriptRequestIdLabel: "Script request ID",
    scriptRequestUnavailable:
      "Script request details are not available for this workstation request.",
    scriptResponseUnavailableErrored:
      "Script response details are unavailable because this workstation request ended with an error.",
    scriptResponseUnavailablePending:
      "Script response details are not available for this workstation request yet.",
    scriptResponseUnavailableSummary:
      "Script response details are not available for this workstation request.",
    selectWorkItemLabel: (workItemLabel: string) =>
      `Select work item ${workItemLabel}`,
    startedAtLabel: "Started at",
    stderrEmpty: "No stderr was recorded for this script response.",
    stderrLabel: "Stderr",
    stdoutEmpty: "No stdout was recorded for this script response.",
    stdoutLabel: "Stdout",
    totalDurationLabel: "Total duration",
    totalDurationUnavailable:
      "Total duration is not available for this workstation request yet.",
    terminalOutcomeLabel: (outcome: string) => {
      switch (outcome.toUpperCase()) {
        case "ACCEPTED":
          return "Accepted";
        case "CONTINUE":
          return "Continue";
        case "FAILED":
          return "Failed";
        case "REJECTED":
          return "Rejected";
        default:
          return outcome;
      }
    },
    terminalRequestContext: ({ outcome, providerSession, workstation }) =>
      providerSession
        ? `${outcome} at ${workstation}; ${providerSession}`
        : `${outcome} at ${workstation}`,
    traceIdsLabel: "Trace IDs",
    traceUnavailable:
      "Trace details are not available for this workstation request yet.",
    transitionIdLabel: "Transition ID",
    worktreeLabel: "Worktree",
    workstationLabel: "Workstation",
    workstationUnavailable:
      "Workstation details are not available for this request.",
    workTypeUnavailable: "Unknown",
    workingDirectoryLabel: "Working directory",
  },
  ja: {
    ...stateNodeDetailFallbackMessages,
    attemptAriaLabel: (attemptNumber: number) =>
      `Inference attempt ${attemptNumber}`,
    attemptTitle: (attemptNumber: number) => `Attempt ${attemptNumber}`,
    collapseAttemptAction: (attemptNumber: number) =>
      `Collapse attempt ${attemptNumber}`,
    collapseRequestBodyAction: "Collapse request body",
    collapseResponseBodyAction: "Collapse response body",
    awaitingProviderResponse: "Awaiting provider response.",
    commandLabel: "Command",
    commandUnavailable:
      "Script command details are not available for this workstation request.",
    consumedPayloadEmpty:
      "No consumed payload content was recorded for this work item.",
    consumedPayloadError:
      "Consumed payload details could not be loaded for this work item.",
    consumedPayloadHeading: "Consumed payload",
    consumedPayloadLoading:
      "Consumed payload details are still loading for this work item.",
    consumedPayloadUnavailable:
      "Consumed payload details are unavailable for this work item.",
    consumedWorkItemsLabel: "Consumed work items",
    dispatchIdLabel: "Dispatch ID",
    durationLabel: "Duration",
    durationUnavailable:
      "Duration details are not available for this script response yet.",
    elapsedTimeLabel: "Elapsed time",
    errorClassLabel: "Error class",
    errorDetailsTitle: "Error details",
    exitCodeLabel: "Exit code",
    exitCodeUnavailable: "Exit code is not available for this script response.",
    failureMessageLabel: "Failure message",
    failureMessageUnavailable:
      "Failure message is not available for this request.",
    failureReasonLabel: "Failure reason",
    failureReasonUnavailable:
      "Failure reason is not available for this request.",
    failureTypeLabel: "Failure type",
    failureTypeUnavailable:
      "Failure type is not available for this script response.",
    inferenceRequestDetailsCopy:
      "Prompt, request payload, working-directory, and worktree details are shown under Inference attempts when available.",
    inferenceRequestIdLabel: "Inference request ID",
    inferenceResponseDetailsCopy:
      "Response, provider-session, and inference metadata details are shown under Inference attempts when available.",
    metadataEmpty:
      "Request metadata is not available for this workstation request.",
    modelLabel: "Model",
    outcomeLabel: "Outcome",
    outcomeUnavailable: "Outcome details are not available yet.",
    pendingOutcome: "PENDING",
    providerLabel: "Provider",
    providerResponseUnavailable:
      "Provider response text is not available for this inference attempt.",
    providerSessionLabel: "Provider session",
    runnerLabel: "Runner",
    runnerCapabilityImageInputLabel: "画像入力",
    runnerCapabilitySessionResumeLabel: "セッション再開",
    runnerCapabilityStructuredOutputLabel: "構造化出力",
    runnerCapabilitySupportHeading: "Runner の機能サポート",
    runnerCapabilitySupportedLabel: "対応",
    runnerCapabilityUnsupportedLabel: "未対応",
    runnerCapabilityWorkingDirectoryLabel: "作業ディレクトリ",
    runnerCapabilityWorktreeLabel: "worktree 選択",
    runnerSelectionSourceLabel: "Runner source",
    collapseAction: "Collapse",
    expandAction: "Expand",
    expandAttemptAction: (attemptNumber: number) =>
      `Expand attempt ${attemptNumber}`,
    expandRequestBodyAction: "Expand request body",
    expandResponseBodyAction: "Expand response body",
    requestBodyLabel: "Request body",
    requestDetailsTitle: "Request details",
    requestIdLabel: "Request ID",
    requestIdUnavailable:
      "Request ID is not available for this workstation request.",
    requestMetadataTitle: "Request metadata",
    requestTimeLabel: "Request time",
    resolvedArgsLabel: "Resolved args",
    responseBodyLabel: "Response body",
    responseDetailsTitle: "Response details",
    responseMetadataTitle: "Response metadata",
    responseMetadataUnavailableErrored:
      "Response metadata is unavailable because this workstation request ended with an error.",
    responseMetadataUnavailableScript:
      "Response metadata is not available for this script-backed workstation request.",
    responseTimeLabel: "Response time",
    timestampUnavailable: "利用不可",
    scriptArgumentsUnavailable:
      "Script arguments are not available for this workstation request.",
    scriptAttemptLabel: "Script attempt",
    scriptAttemptUnavailable: "Script attempt is not available yet.",
    scriptRequestIdLabel: "Script request ID",
    scriptRequestUnavailable:
      "Script request details are not available for this workstation request.",
    scriptResponseUnavailableErrored:
      "Script response details are unavailable because this workstation request ended with an error.",
    scriptResponseUnavailablePending:
      "Script response details are not available for this workstation request yet.",
    scriptResponseUnavailableSummary:
      "Script response details are not available for this workstation request.",
    selectWorkItemLabel: (workItemLabel: string) =>
      `Select work item ${workItemLabel}`,
    startedAtLabel: "開始時刻",
    stderrEmpty: "No stderr was recorded for this script response.",
    stderrLabel: "Stderr",
    stdoutEmpty: "No stdout was recorded for this script response.",
    stdoutLabel: "Stdout",
    totalDurationLabel: "Total duration",
    totalDurationUnavailable:
      "Total duration is not available for this workstation request yet.",
    terminalOutcomeLabel: (outcome: string) => {
      switch (outcome.toUpperCase()) {
        case "ACCEPTED":
          return "Accepted";
        case "CONTINUE":
          return "Continue";
        case "FAILED":
          return "Failed";
        case "REJECTED":
          return "Rejected";
        default:
          return outcome;
      }
    },
    terminalRequestContext: ({ outcome, providerSession, workstation }) =>
      providerSession
        ? `${outcome} at ${workstation}; ${providerSession}`
        : `${outcome} at ${workstation}`,
    traceIdsLabel: "Trace IDs",
    traceUnavailable:
      "Trace details are not available for this workstation request yet.",
    transitionIdLabel: "Transition ID",
    worktreeLabel: "Worktree",
    workstationLabel: "Workstation",
    workstationUnavailable:
      "Workstation details are not available for this request.",
    workTypeUnavailable: "알 수 없음",
    workingDirectoryLabel: "Working directory",
  },
  ko: {
    ...stateNodeDetailFallbackMessages,
    attemptAriaLabel: (attemptNumber: number) =>
      `Inference attempt ${attemptNumber}`,
    attemptTitle: (attemptNumber: number) => `Attempt ${attemptNumber}`,
    collapseAttemptAction: (attemptNumber: number) =>
      `Collapse attempt ${attemptNumber}`,
    collapseRequestBodyAction: "Collapse request body",
    collapseResponseBodyAction: "Collapse response body",
    awaitingProviderResponse: "Awaiting provider response.",
    commandLabel: "Command",
    commandUnavailable:
      "Script command details are not available for this workstation request.",
    consumedPayloadEmpty:
      "No consumed payload content was recorded for this work item.",
    consumedPayloadError:
      "Consumed payload details could not be loaded for this work item.",
    consumedPayloadHeading: "Consumed payload",
    consumedPayloadLoading:
      "Consumed payload details are still loading for this work item.",
    consumedPayloadUnavailable:
      "Consumed payload details are unavailable for this work item.",
    consumedWorkItemsLabel: "Consumed work items",
    dispatchIdLabel: "Dispatch ID",
    durationLabel: "Duration",
    durationUnavailable:
      "Duration details are not available for this script response yet.",
    elapsedTimeLabel: "Elapsed time",
    errorClassLabel: "Error class",
    errorDetailsTitle: "Error details",
    exitCodeLabel: "Exit code",
    exitCodeUnavailable: "Exit code is not available for this script response.",
    failureMessageLabel: "Failure message",
    failureMessageUnavailable:
      "Failure message is not available for this request.",
    failureReasonLabel: "Failure reason",
    failureReasonUnavailable:
      "Failure reason is not available for this request.",
    failureTypeLabel: "Failure type",
    failureTypeUnavailable:
      "Failure type is not available for this script response.",
    inferenceRequestDetailsCopy:
      "Prompt, request payload, working-directory, and worktree details are shown under Inference attempts when available.",
    inferenceRequestIdLabel: "Inference request ID",
    inferenceResponseDetailsCopy:
      "Response, provider-session, and inference metadata details are shown under Inference attempts when available.",
    metadataEmpty:
      "Request metadata is not available for this workstation request.",
    modelLabel: "Model",
    outcomeLabel: "Outcome",
    outcomeUnavailable: "Outcome details are not available yet.",
    pendingOutcome: "PENDING",
    providerLabel: "Provider",
    providerResponseUnavailable:
      "Provider response text is not available for this inference attempt.",
    providerSessionLabel: "Provider session",
    runnerLabel: "Runner",
    runnerCapabilityImageInputLabel: "이미지 입력",
    runnerCapabilitySessionResumeLabel: "세션 재개",
    runnerCapabilityStructuredOutputLabel: "구조화된 출력",
    runnerCapabilitySupportHeading: "Runner 기능 지원",
    runnerCapabilitySupportedLabel: "지원",
    runnerCapabilityUnsupportedLabel: "미지원",
    runnerCapabilityWorkingDirectoryLabel: "작업 디렉터리",
    runnerCapabilityWorktreeLabel: "worktree 선택",
    runnerSelectionSourceLabel: "Runner source",
    collapseAction: "Collapse",
    expandAction: "Expand",
    expandAttemptAction: (attemptNumber: number) =>
      `Expand attempt ${attemptNumber}`,
    expandRequestBodyAction: "Expand request body",
    expandResponseBodyAction: "Expand response body",
    requestBodyLabel: "Request body",
    requestDetailsTitle: "Request details",
    requestIdLabel: "Request ID",
    requestIdUnavailable:
      "Request ID is not available for this workstation request.",
    requestMetadataTitle: "Request metadata",
    requestTimeLabel: "Request time",
    resolvedArgsLabel: "Resolved args",
    responseBodyLabel: "Response body",
    responseDetailsTitle: "Response details",
    responseMetadataTitle: "Response metadata",
    responseMetadataUnavailableErrored:
      "Response metadata is unavailable because this workstation request ended with an error.",
    responseMetadataUnavailableScript:
      "Response metadata is not available for this script-backed workstation request.",
    responseTimeLabel: "Response time",
    timestampUnavailable: "사용할 수 없음",
    scriptArgumentsUnavailable:
      "Script arguments are not available for this workstation request.",
    scriptAttemptLabel: "Script attempt",
    scriptAttemptUnavailable: "Script attempt is not available yet.",
    scriptRequestIdLabel: "Script request ID",
    scriptRequestUnavailable:
      "Script request details are not available for this workstation request.",
    scriptResponseUnavailableErrored:
      "Script response details are unavailable because this workstation request ended with an error.",
    scriptResponseUnavailablePending:
      "Script response details are not available for this workstation request yet.",
    scriptResponseUnavailableSummary:
      "Script response details are not available for this workstation request.",
    selectWorkItemLabel: (workItemLabel: string) =>
      `Select work item ${workItemLabel}`,
    startedAtLabel: "시작 시각",
    stderrEmpty: "No stderr was recorded for this script response.",
    stderrLabel: "Stderr",
    stdoutEmpty: "No stdout was recorded for this script response.",
    stdoutLabel: "Stdout",
    totalDurationLabel: "Total duration",
    totalDurationUnavailable:
      "Total duration is not available for this workstation request yet.",
    terminalOutcomeLabel: (outcome: string) => {
      switch (outcome.toUpperCase()) {
        case "ACCEPTED":
          return "Accepted";
        case "CONTINUE":
          return "Continue";
        case "FAILED":
          return "Failed";
        case "REJECTED":
          return "Rejected";
        default:
          return outcome;
      }
    },
    terminalRequestContext: ({ outcome, providerSession, workstation }) =>
      providerSession
        ? `${outcome} at ${workstation}; ${providerSession}`
        : `${outcome} at ${workstation}`,
    traceIdsLabel: "Trace IDs",
    traceUnavailable:
      "Trace details are not available for this workstation request yet.",
    transitionIdLabel: "Transition ID",
    worktreeLabel: "Worktree",
    workstationLabel: "Workstation",
    workstationUnavailable:
      "Workstation details are not available for this request.",
    workingDirectoryLabel: "Working directory",
  },
  "zh-CN": {
    attemptAriaLabel: (attemptNumber: number) => `推理尝试 ${attemptNumber}`,
    attemptTitle: (attemptNumber: number) => `尝试 ${attemptNumber}`,
    collapseAttemptAction: (attemptNumber: number) =>
      `收起尝试 ${attemptNumber}`,
    collapseRequestBodyAction: "收起请求正文",
    collapseResponseBodyAction: "收起响应正文",
    awaitingProviderResponse: "正在等待提供方响应。",
    commandLabel: "命令",
    commandUnavailable: "此工作站请求没有可用的脚本命令详情。",
    consumedPayloadEmpty: "此工作项没有记录已消费的负载内容。",
    consumedPayloadError: "此工作项的已消费负载详情暂时无法加载。",
    consumedPayloadHeading: "已消费的负载",
    consumedPayloadLoading: "此工作项的已消费负载详情仍在加载。",
    consumedPayloadUnavailable: "此工作项的已消费负载详情不可用。",
    countLabel: "数量",
    consumedWorkItemsLabel: "已消费的工作项",
    currentWorkHeading: "当前工作",
    dispatchIdLabel: "分派 ID",
    durationLabel: "耗时",
    durationUnavailable: "此脚本响应的耗时详情暂不可用。",
    elapsedTimeLabel: "耗时",
    errorClassLabel: "错误类别",
    errorDetailsTitle: "错误详情",
    exitCodeLabel: "退出码",
    exitCodeUnavailable: "此脚本响应没有可用的退出码。",
    failureMessageLabel: "失败消息",
    failureMessageUnavailable: "此请求没有可用的失败消息。",
    failureReasonLabel: "失败原因",
    failureReasonUnavailable: "此请求没有可用的失败原因。",
    failureTypeLabel: "失败类型",
    failureTypeUnavailable: "此脚本响应没有可用的失败类型。",
    inferenceRequestDetailsCopy:
      "提示词、请求负载、工作目录和工作树详情会在可用时显示在“推理尝试”下方。",
    inferenceRequestIdLabel: "推理请求 ID",
    inferenceResponseDetailsCopy:
      "响应、provider-session 和推理元数据详情会在可用时显示在“推理尝试”下方。",
    metadataEmpty: "此工作站请求没有可用的请求元数据。",
    modelLabel: "模型",
    outcomeLabel: "结果",
    outcomeUnavailable: "结果详情暂不可用。",
    pendingOutcome: "等待中",
    providerLabel: "提供方",
    providerResponseUnavailable: "此推理尝试没有可用的提供方响应文本。",
    providerSessionLabel: "Provider session",
    runnerLabel: "Runner",
    runnerCapabilityImageInputLabel: "图片输入",
    runnerCapabilitySessionResumeLabel: "会话恢复",
    runnerCapabilityStructuredOutputLabel: "结构化输出",
    runnerCapabilitySupportHeading: "Runner 能力支持",
    runnerCapabilitySupportedLabel: "支持",
    runnerCapabilityUnsupportedLabel: "不支持",
    runnerCapabilityWorkingDirectoryLabel: "工作目录",
    runnerCapabilityWorktreeLabel: "worktree 选择",
    runnerSelectionSourceLabel: "Runner 来源",
    collapseAction: "折叠",
    expandAction: "展开",
    expandAttemptAction: (attemptNumber: number) => `展开尝试 ${attemptNumber}`,
    expandRequestBodyAction: "展开请求正文",
    expandResponseBodyAction: "展开响应正文",
    noCurrentWorkInPlace: "当前没有工作占用这个位置。",
    noWorkRecordedAtSelectedTick:
      "在所选时间刻度，这个位置暂时没有记录到工作。",
    requestBodyLabel: "请求正文",
    requestDetailsTitle: "请求详情",
    requestIdLabel: "请求 ID",
    requestIdUnavailable: "此工作站请求没有可用的请求 ID。",
    requestMetadataTitle: "请求元数据",
    requestTimeLabel: "请求时间",
    resolvedArgsLabel: "已解析参数",
    responseBodyLabel: "响应正文",
    responseDetailsTitle: "响应详情",
    responseMetadataTitle: "响应元数据",
    responseMetadataUnavailableErrored:
      "此工作站请求因错误结束，因此响应元数据不可用。",
    responseMetadataUnavailableScript:
      "此脚本驱动的工作站请求没有可用的响应元数据。",
    responseTimeLabel: "响应时间",
    timestampUnavailable: "不可用",
    selectedTickWorkUnavailable:
      "在所选时间刻度，这个位置对应的工作暂时不可用。",
    scriptArgumentsUnavailable: "此工作站请求没有可用的脚本参数。",
    scriptAttemptLabel: "脚本尝试",
    scriptAttemptUnavailable: "脚本尝试暂不可用。",
    scriptRequestIdLabel: "脚本请求 ID",
    scriptRequestUnavailable: "此工作站请求没有可用的脚本请求详情。",
    scriptResponseUnavailableErrored:
      "此工作站请求因错误结束，因此脚本响应详情不可用。",
    scriptResponseUnavailablePending: "此工作站请求的脚本响应详情暂不可用。",
    scriptResponseUnavailableSummary: "此工作站请求没有可用的脚本响应详情。",
    selectWorkItemLabel: (workItemLabel: string) =>
      `选择工作项 ${workItemLabel}`,
    startedAtLabel: "开始时间",
    stderrEmpty: "此脚本响应没有记录 stderr。",
    stderrLabel: "标准错误",
    stdoutEmpty: "此脚本响应没有记录 stdout。",
    stdoutLabel: "标准输出",
    stateLabel: "状态",
    stateNodeIdLabel: "状态节点 ID",
    totalDurationLabel: "总耗时",
    totalDurationUnavailable: "此工作站请求的总耗时暂不可用。",
    terminalOutcomeLabel: (outcome: string) => {
      switch (outcome.toUpperCase()) {
        case "ACCEPTED":
          return "已接受";
        case "CONTINUE":
          return "继续";
        case "FAILED":
          return "失败";
        case "REJECTED":
          return "已拒绝";
        default:
          return outcome;
      }
    },
    terminalRequestContext: ({ outcome, providerSession, workstation }) =>
      providerSession
        ? `${outcome} 于 ${workstation}; ${providerSession}`
        : `${outcome} 于 ${workstation}`,
    traceIdLabel: "追踪 ID",
    traceIdsLabel: "追踪 ID",
    traceUnavailable: "此工作站请求的追踪详情暂不可用。",
    transitionIdLabel: "转换 ID",
    workIdLabel: "工作 ID",
    worktreeLabel: "工作树",
    workstationLabel: "工作站",
    workstationUnavailable: "此请求没有可用的工作站详情。",
    workTypeLabel: "工作类型",
    workTypeUnavailable: "未知",
    workingDirectoryLabel: "工作目录",
  },
} satisfies LocalizedMessages<CurrentSelectionDetailMessages>;

export const getCurrentSelectionDetailMessages = (locale?: string | null) =>
  resolveLocalizedMessages(currentSelectionDetailMessagesByLocale, locale);

export { currentSelectionDetailMessagesByLocale };
