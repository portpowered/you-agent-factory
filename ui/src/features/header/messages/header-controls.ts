import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface HeaderControlsMessages {
  activeSessionPathLabel: string;
  brandWordmark: string;
  currentTickStatusTemplate: string;
  dashboardSummaryLabel: string;
  dashboardUnavailableTitle: string;
  languageLabel: string;
  languageMenuButtonLabel: string;
  loadingSessionsLabel: string;
  loadingDashboardTitle: string;
  openSessionButtonLabel: string;
  openSessionDialogDescription: string;
  openSessionDialogTitle: string;
  openSessionSubmitLabel: string;
  openSessionSubmitPendingLabel: string;
  returnToCurrentTickLabel: string;
  retrySessionsLabel: string;
  selectSessionTargetLabel: string;
  sessionFolderFieldLabel: string;
  sessionFolderFieldPlaceholder: string;
  sessionTabsLabel: string;
  sessionsEmptyTitle: string;
  sessionsErrorTitle: string;
  sessionsOfflineTitle: string;
  sliderAriaLabel: string;
  sliderLabel: string;
  streamStatusConnectingLabel: string;
  streamStatusLiveLabel: string;
  streamStatusOfflineLabel: string;
  targetPickerHint: string;
  targetPickerTitle: string;
  waitingForMoreTicks: string;
}

export const HEADER_CURRENT_TICK_TOKEN = "{{currentTick}}";
export const HEADER_MAX_TICK_TOKEN = "{{maxTick}}";

const headerControlsMessagesByLocale = {
  en: {
    activeSessionPathLabel: "Active folder",
    brandWordmark: "Infinite You",
    currentTickStatusTemplate: `Tick ${HEADER_CURRENT_TICK_TOKEN} of ${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "dashboard summary",
    dashboardUnavailableTitle: "Dashboard unavailable",
    languageLabel: "Language",
    languageMenuButtonLabel: "Change language",
    loadingSessionsLabel: "Loading sessions...",
    loadingDashboardTitle: "Loading dashboard",
    openSessionButtonLabel: "Open another session",
    openSessionDialogDescription:
      "Open another live factory session from a folder path and, when needed, pick one runnable target.",
    openSessionDialogTitle: "Open factory session",
    openSessionSubmitLabel: "Inspect folder",
    openSessionSubmitPendingLabel: "Inspecting folder...",
    returnToCurrentTickLabel: "Return to current tick",
    retrySessionsLabel: "Retry sessions",
    selectSessionTargetLabel: "Open session target",
    sessionFolderFieldLabel: "Factory folder",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabsLabel: "factory sessions",
    sessionsEmptyTitle: "No live sessions",
    sessionsErrorTitle: "Factory sessions unavailable",
    sessionsOfflineTitle: "Factory sessions offline",
    sliderAriaLabel: "Timeline tick",
    sliderLabel: "Timeline tick",
    streamStatusConnectingLabel: "Infinite You event stream connecting",
    streamStatusLiveLabel: "Infinite You event stream live",
    streamStatusOfflineLabel: "Infinite You event stream offline",
    targetPickerHint: "Choose one runnable target from this folder.",
    targetPickerTitle: "Pick a runnable target",
    waitingForMoreTicks: "Waiting for more ticks",
  },
  ja: {
    activeSessionPathLabel: "現在のフォルダー",
    brandWordmark: "Infinite You",
    currentTickStatusTemplate: `${HEADER_MAX_TICK_TOKEN} 件中 ${HEADER_CURRENT_TICK_TOKEN} 件目のティック`,
    dashboardSummaryLabel: "ダッシュボードの概要",
    dashboardUnavailableTitle: "ダッシュボードを利用できません",
    languageLabel: "言語",
    languageMenuButtonLabel: "言語を変更",
    loadingSessionsLabel: "セッションを読み込み中...",
    loadingDashboardTitle: "ダッシュボードを読み込み中",
    openSessionButtonLabel: "別のセッションを開く",
    openSessionDialogDescription:
      "フォルダーのパスから別の実行中セッションを開き、必要な場合は実行可能なターゲットを選択します。",
    openSessionDialogTitle: "ファクトリーセッションを開く",
    openSessionSubmitLabel: "フォルダーを確認",
    openSessionSubmitPendingLabel: "フォルダーを確認中...",
    returnToCurrentTickLabel: "現在のティックに戻る",
    retrySessionsLabel: "セッションを再試行",
    selectSessionTargetLabel: "セッションターゲットを開く",
    sessionFolderFieldLabel: "ファクトリーフォルダー",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabsLabel: "ファクトリーセッション",
    sessionsEmptyTitle: "実行中のセッションはありません",
    sessionsErrorTitle: "ファクトリーセッションを表示できません",
    sessionsOfflineTitle: "ファクトリーセッションはオフラインです",
    sliderAriaLabel: "タイムラインティック",
    sliderLabel: "タイムラインティック",
    streamStatusConnectingLabel: "Infinite You のイベントストリームを接続中",
    streamStatusLiveLabel: "Infinite You のイベントストリームはライブです",
    streamStatusOfflineLabel:
      "Infinite You のイベントストリームはオフラインです",
    targetPickerHint: "このフォルダーから実行可能なターゲットを 1 つ選択してください。",
    targetPickerTitle: "実行可能なターゲットを選択",
    waitingForMoreTicks: "ティックが増えるまで待機しています",
  },
  ko: {
    activeSessionPathLabel: "활성 폴더",
    brandWordmark: "Infinite You",
    currentTickStatusTemplate: `틱 ${HEADER_CURRENT_TICK_TOKEN} / ${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "대시보드 요약",
    dashboardUnavailableTitle: "대시보드를 사용할 수 없음",
    languageLabel: "언어",
    languageMenuButtonLabel: "언어 변경",
    loadingSessionsLabel: "세션을 불러오는 중...",
    loadingDashboardTitle: "대시보드 로드 중",
    openSessionButtonLabel: "다른 세션 열기",
    openSessionDialogDescription:
      "폴더 경로에서 다른 라이브 팩토리 세션을 열고, 필요하면 실행 가능한 대상을 선택합니다.",
    openSessionDialogTitle: "팩토리 세션 열기",
    openSessionSubmitLabel: "폴더 확인",
    openSessionSubmitPendingLabel: "폴더를 확인하는 중...",
    returnToCurrentTickLabel: "현재 틱으로 돌아가기",
    retrySessionsLabel: "세션 다시 시도",
    selectSessionTargetLabel: "세션 대상 열기",
    sessionFolderFieldLabel: "팩토리 폴더",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabsLabel: "팩토리 세션",
    sessionsEmptyTitle: "실행 중인 세션이 없습니다",
    sessionsErrorTitle: "팩토리 세션을 불러올 수 없음",
    sessionsOfflineTitle: "팩토리 세션이 오프라인입니다",
    sliderAriaLabel: "타임라인 틱",
    sliderLabel: "타임라인 틱",
    streamStatusConnectingLabel: "Infinite You 이벤트 스트림에 연결 중",
    streamStatusLiveLabel: "Infinite You 이벤트 스트림이 라이브 상태입니다",
    streamStatusOfflineLabel:
      "Infinite You 이벤트 스트림이 오프라인 상태입니다",
    targetPickerHint: "이 폴더에서 실행 가능한 대상을 하나 선택하세요.",
    targetPickerTitle: "실행 가능한 대상 선택",
    waitingForMoreTicks: "틱이 더 쌓일 때까지 기다리는 중",
  },
  "zh-CN": {
    activeSessionPathLabel: "当前文件夹",
    brandWordmark: "Infinite You",
    currentTickStatusTemplate: `第 ${HEADER_CURRENT_TICK_TOKEN} 个刻度，共 ${HEADER_MAX_TICK_TOKEN} 个`,
    dashboardSummaryLabel: "仪表板概览",
    dashboardUnavailableTitle: "仪表板不可用",
    languageLabel: "语言",
    languageMenuButtonLabel: "切换语言",
    loadingSessionsLabel: "正在加载会话...",
    loadingDashboardTitle: "正在加载仪表板",
    openSessionButtonLabel: "打开另一个会话",
    openSessionDialogDescription:
      "从文件夹路径打开另一个运行中的工厂会话，并在需要时选择可运行目标。",
    openSessionDialogTitle: "打开工厂会话",
    openSessionSubmitLabel: "检查文件夹",
    openSessionSubmitPendingLabel: "正在检查文件夹...",
    returnToCurrentTickLabel: "返回当前刻度",
    retrySessionsLabel: "重试会话",
    selectSessionTargetLabel: "打开会话目标",
    sessionFolderFieldLabel: "工厂文件夹",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabsLabel: "工厂会话",
    sessionsEmptyTitle: "没有运行中的会话",
    sessionsErrorTitle: "工厂会话不可用",
    sessionsOfflineTitle: "工厂会话离线",
    sliderAriaLabel: "时间线刻度",
    sliderLabel: "时间线刻度",
    streamStatusConnectingLabel: "Infinite You 事件流正在连接",
    streamStatusLiveLabel: "Infinite You 事件流在线",
    streamStatusOfflineLabel: "Infinite You 事件流离线",
    targetPickerHint: "请选择此文件夹中的一个可运行目标。",
    targetPickerTitle: "选择可运行目标",
    waitingForMoreTicks: "正在等待更多刻度",
  },
} satisfies LocalizedMessages<HeaderControlsMessages>;

export function getHeaderControlsMessages(
  locale?: string | null,
): HeaderControlsMessages {
  return resolveLocalizedMessages(headerControlsMessagesByLocale, locale);
}

export { headerControlsMessagesByLocale };
