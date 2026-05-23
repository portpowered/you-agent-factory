import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface HeaderControlsMessages {
  brandWordmark: string;
  browseSessionFolderButtonLabel: string;
  closingSessionButtonLabel: string;
  currentTickStatusTemplate: string;
  dashboardSummaryLabel: string;
  dashboardUnavailableTitle: string;
  globalHeaderActionsLabel: string;
  languageLabel: string;
  languageMenuButtonLabel: string;
  loadingSessionsLabel: string;
  loadingDashboardTitle: string;
  openSessionButtonLabel: string;
  openSessionDialogDescription: string;
  openSessionFolderRequiredError: string;
  openSessionFolderMissingError: string;
  openSessionFolderNotDirectoryError: string;
  openSessionFolderNotRunnableError: string;
  openSessionFolderUnreadableError: string;
  openSessionFolderUnknownError: string;
  openSessionLaunchReadyMultipleTargets: string;
  openSessionLaunchReadySingleTarget: string;
  openSessionDialogTitle: string;
  openSessionSubmitLabel: string;
  openSessionSubmitPendingLabel: string;
  pauseSessionStreamLabelTemplate: string;
  openSessionValidationPendingLabel: string;
  returnToCurrentTickLabel: string;
  resumeSessionStreamLabelTemplate: string;
  retrySessionsLabel: string;
  selectSessionTargetLabel: string;
  sessionFolderFieldLabel: string;
  sessionFolderHelperText: string;
  sessionFolderFieldPlaceholder: string;
  sessionTabCloseLabelTemplate: string;
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
    brandWordmark: "you-agent-factory",
    browseSessionFolderButtonLabel: "Choose folder",
    closingSessionButtonLabel: "Closing session",
    currentTickStatusTemplate: `${HEADER_CURRENT_TICK_TOKEN}/${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "dashboard summary",
    dashboardUnavailableTitle: "Dashboard unavailable",
    globalHeaderActionsLabel: "Dashboard actions",
    languageLabel: "Language",
    languageMenuButtonLabel: "Change language",
    loadingSessionsLabel: "Loading sessions...",
    loadingDashboardTitle: "Loading dashboard",
    openSessionButtonLabel: "Open factory folder",
    openSessionDialogDescription:
      "Choose or enter a local factory folder. The folder must contain a runnable factory before you can open a session.",
    openSessionFolderRequiredError:
      "Enter a local factory folder path, then check the folder before launch.",
    openSessionFolderMissingError:
      "This folder does not exist yet. Check the path and choose an existing local factory folder.",
    openSessionFolderNotDirectoryError:
      "This path points to a file instead of a folder. Choose a local factory folder that contains a runnable factory.",
    openSessionFolderNotRunnableError:
      "This folder was found, but it does not contain a runnable factory. Choose a folder with a runnable factory and try again.",
    openSessionFolderUnreadableError:
      "This folder could not be read from this machine. Check its permissions, then choose a readable local factory folder.",
    openSessionFolderUnknownError:
      "The dashboard could not verify this factory folder. Check the path and try again.",
    openSessionLaunchReadyMultipleTargets:
      "Folder is ready. Choose one runnable target to continue.",
    openSessionLaunchReadySingleTarget:
      "Folder is ready. Open the runnable target shown below to continue.",
    openSessionDialogTitle: "Open a factory folder",
    openSessionSubmitLabel: "Check folder",
    openSessionSubmitPendingLabel: "Checking folder...",
    pauseSessionStreamLabelTemplate: "Pause {{sessionLabel}} updates",
    openSessionValidationPendingLabel:
      "Checking whether this folder contains a runnable factory...",
    pauseSessionStreamLabelTemplate: "Pause {{sessionLabel}} updates",
    returnToCurrentTickLabel: "Return to current tick",
    resumeSessionStreamLabelTemplate: "Resume {{sessionLabel}} updates",
    retrySessionsLabel: "Retry sessions",
    selectSessionTargetLabel: "Open session target",
    sessionFolderFieldLabel: "Factory folder",
    sessionFolderHelperText:
      "Enter a local factory folder path or choose a folder from your machine. The folder must contain a runnable factory.",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabCloseLabelTemplate: "Close {{sessionLabel}} session",
    sessionTabsLabel: "factory sessions",
    sessionsEmptyTitle: "No live sessions",
    sessionsErrorTitle: "Factory sessions unavailable",
    sessionsOfflineTitle: "Factory sessions offline",
    sliderAriaLabel: "Timeline tick",
    sliderLabel: "Timeline tick",
    streamStatusConnectingLabel: "you-agent-factory event stream connecting",
    streamStatusLiveLabel: "you-agent-factory event stream live",
    streamStatusOfflineLabel: "you-agent-factory event stream offline",
    targetPickerHint: "Choose one runnable target from this folder.",
    targetPickerTitle: "Pick a runnable target",
    waitingForMoreTicks: "Waiting for more ticks",
  },
  ja: {
    brandWordmark: "you-agent-factory",
    browseSessionFolderButtonLabel: "フォルダーを選ぶ",
    closingSessionButtonLabel: "セッションを終了中",
    currentTickStatusTemplate: `${HEADER_CURRENT_TICK_TOKEN}/${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "ダッシュボードの概要",
    dashboardUnavailableTitle: "ダッシュボードを利用できません",
    globalHeaderActionsLabel: "ダッシュボードの操作",
    languageLabel: "言語",
    languageMenuButtonLabel: "言語を変更",
    loadingSessionsLabel: "セッションを読み込み中...",
    loadingDashboardTitle: "ダッシュボードを読み込み中",
    openSessionButtonLabel: "ファクトリーフォルダーを開く",
    openSessionDialogDescription:
      "ローカルのファクトリーフォルダーを選ぶか入力してください。セッションを開くには、そのフォルダーに実行可能なファクトリーが含まれている必要があります。",
    openSessionFolderRequiredError:
      "ローカルのファクトリーフォルダーのパスを入力してから、起動前にフォルダーを確認してください。",
    openSessionFolderMissingError:
      "このフォルダーはまだ存在しません。パスを確認して、既存のローカルファクトリーフォルダーを選んでください。",
    openSessionFolderNotDirectoryError:
      "このパスはフォルダーではなくファイルを指しています。実行可能なファクトリーを含むローカルファクトリーフォルダーを選んでください。",
    openSessionFolderNotRunnableError:
      "このフォルダーは見つかりましたが、実行可能なファクトリーが含まれていません。実行可能なファクトリーを含むフォルダーを選んで、もう一度試してください。",
    openSessionFolderUnreadableError:
      "このフォルダーをこの端末から読み取れませんでした。権限を確認してから、読み取れるローカルファクトリーフォルダーを選んでください。",
    openSessionFolderUnknownError:
      "このファクトリーフォルダーをダッシュボードで確認できませんでした。パスを確認して、もう一度試してください。",
    openSessionLaunchReadyMultipleTargets:
      "フォルダーを確認できました。続行するには実行可能なターゲットを 1 つ選んでください。",
    openSessionLaunchReadySingleTarget:
      "フォルダーを確認できました。続行するには下に表示された実行可能なターゲットを開いてください。",
    openSessionDialogTitle: "ファクトリーフォルダーを開く",
    openSessionSubmitLabel: "フォルダーを確認する",
    openSessionSubmitPendingLabel: "フォルダーを確認しています...",
    pauseSessionStreamLabelTemplate: "{{sessionLabel}} の更新を一時停止",
    openSessionValidationPendingLabel:
      "このフォルダーに実行可能なファクトリーがあるか確認しています...",
    pauseSessionStreamLabelTemplate: "{{sessionLabel}} の更新を一時停止",
    returnToCurrentTickLabel: "現在のティックに戻る",
    resumeSessionStreamLabelTemplate: "{{sessionLabel}} の更新を再開",
    retrySessionsLabel: "セッションを再試行",
    selectSessionTargetLabel: "セッションターゲットを開く",
    sessionFolderFieldLabel: "ファクトリーフォルダー",
    sessionFolderHelperText:
      "ローカルのファクトリーフォルダーのパスを入力するか、この端末からフォルダーを選んでください。フォルダーには実行可能なファクトリーが含まれている必要があります。",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabCloseLabelTemplate: "{{sessionLabel}} セッションを閉じる",
    sessionTabsLabel: "ファクトリーセッション",
    sessionsEmptyTitle: "実行中のセッションはありません",
    sessionsErrorTitle: "ファクトリーセッションを表示できません",
    sessionsOfflineTitle: "ファクトリーセッションはオフラインです",
    sliderAriaLabel: "タイムラインティック",
    sliderLabel: "タイムラインティック",
    streamStatusConnectingLabel: "you-agent-factory のイベントストリームを接続中",
    streamStatusLiveLabel: "you-agent-factory のイベントストリームはライブです",
    streamStatusOfflineLabel:
      "you-agent-factory のイベントストリームはオフラインです",
    targetPickerHint: "このフォルダーから実行可能なターゲットを 1 つ選択してください。",
    targetPickerTitle: "実行可能なターゲットを選択",
    waitingForMoreTicks: "ティックが増えるまで待機しています",
  },
  ko: {
    brandWordmark: "you-agent-factory",
    browseSessionFolderButtonLabel: "폴더 선택",
    closingSessionButtonLabel: "세션 종료 중",
    currentTickStatusTemplate: `${HEADER_CURRENT_TICK_TOKEN}/${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "대시보드 요약",
    dashboardUnavailableTitle: "대시보드를 사용할 수 없음",
    globalHeaderActionsLabel: "대시보드 작업",
    languageLabel: "언어",
    languageMenuButtonLabel: "언어 변경",
    loadingSessionsLabel: "세션을 불러오는 중...",
    loadingDashboardTitle: "대시보드 로드 중",
    openSessionButtonLabel: "팩토리 폴더 열기",
    openSessionDialogDescription:
      "로컬 팩토리 폴더를 선택하거나 경로를 입력하세요. 세션을 열려면 해당 폴더에 실행 가능한 팩토리가 있어야 합니다.",
    openSessionFolderRequiredError:
      "로컬 팩토리 폴더 경로를 입력한 다음 시작 전에 폴더를 확인하세요.",
    openSessionFolderMissingError:
      "이 폴더는 아직 존재하지 않습니다. 경로를 확인하고 기존 로컬 팩토리 폴더를 선택하세요.",
    openSessionFolderNotDirectoryError:
      "이 경로는 폴더가 아니라 파일을 가리킵니다. 실행 가능한 팩토리가 들어 있는 로컬 팩토리 폴더를 선택하세요.",
    openSessionFolderNotRunnableError:
      "이 폴더는 찾았지만 실행 가능한 팩토리가 들어 있지 않습니다. 실행 가능한 팩토리가 있는 폴더를 선택한 뒤 다시 시도하세요.",
    openSessionFolderUnreadableError:
      "이 기기에서 이 폴더를 읽을 수 없습니다. 권한을 확인한 다음 읽을 수 있는 로컬 팩토리 폴더를 선택하세요.",
    openSessionFolderUnknownError:
      "대시보드에서 이 팩토리 폴더를 확인할 수 없습니다. 경로를 확인한 뒤 다시 시도하세요.",
    openSessionLaunchReadyMultipleTargets:
      "폴더를 확인했습니다. 계속하려면 실행 가능한 대상을 하나 선택하세요.",
    openSessionLaunchReadySingleTarget:
      "폴더를 확인했습니다. 계속하려면 아래에 표시된 실행 가능한 대상을 여세요.",
    openSessionDialogTitle: "팩토리 폴더 열기",
    openSessionSubmitLabel: "폴더 확인",
    openSessionSubmitPendingLabel: "폴더 확인 중...",
    pauseSessionStreamLabelTemplate: "{{sessionLabel}} 업데이트 일시중지",
    openSessionValidationPendingLabel:
      "이 폴더에 실행 가능한 팩토리가 있는지 확인하는 중...",
    pauseSessionStreamLabelTemplate: "{{sessionLabel}} 업데이트 일시중지",
    returnToCurrentTickLabel: "현재 틱으로 돌아가기",
    resumeSessionStreamLabelTemplate: "{{sessionLabel}} 업데이트 다시 시작",
    retrySessionsLabel: "세션 다시 시도",
    selectSessionTargetLabel: "세션 대상 열기",
    sessionFolderFieldLabel: "팩토리 폴더",
    sessionFolderHelperText:
      "로컬 팩토리 폴더 경로를 입력하거나 이 기기에서 폴더를 선택하세요. 폴더에는 실행 가능한 팩토리가 있어야 합니다.",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabCloseLabelTemplate: "{{sessionLabel}} 세션 닫기",
    sessionTabsLabel: "팩토리 세션",
    sessionsEmptyTitle: "실행 중인 세션이 없습니다",
    sessionsErrorTitle: "팩토리 세션을 불러올 수 없음",
    sessionsOfflineTitle: "팩토리 세션이 오프라인입니다",
    sliderAriaLabel: "타임라인 틱",
    sliderLabel: "타임라인 틱",
    streamStatusConnectingLabel: "you-agent-factory 이벤트 스트림에 연결 중",
    streamStatusLiveLabel: "you-agent-factory 이벤트 스트림이 라이브 상태입니다",
    streamStatusOfflineLabel:
      "you-agent-factory 이벤트 스트림이 오프라인 상태입니다",
    targetPickerHint: "이 폴더에서 실행 가능한 대상을 하나 선택하세요.",
    targetPickerTitle: "실행 가능한 대상 선택",
    waitingForMoreTicks: "틱이 더 쌓일 때까지 기다리는 중",
  },
  "zh-CN": {
    brandWordmark: "you-agent-factory",
    browseSessionFolderButtonLabel: "选择文件夹",
    closingSessionButtonLabel: "正在关闭会话",
    currentTickStatusTemplate: `${HEADER_CURRENT_TICK_TOKEN}/${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "仪表板概览",
    dashboardUnavailableTitle: "仪表板不可用",
    globalHeaderActionsLabel: "仪表板操作",
    languageLabel: "语言",
    languageMenuButtonLabel: "切换语言",
    loadingSessionsLabel: "正在加载会话...",
    loadingDashboardTitle: "正在加载仪表板",
    openSessionButtonLabel: "打开工厂文件夹",
    openSessionDialogDescription:
      "请选择或输入本地工厂文件夹。只有文件夹中包含可运行的工厂时，才能打开会话。",
    openSessionFolderRequiredError:
      "请输入本地工厂文件夹路径，然后在启动前检查该文件夹。",
    openSessionFolderMissingError:
      "此文件夹尚不存在。请检查路径并选择一个已存在的本地工厂文件夹。",
    openSessionFolderNotDirectoryError:
      "此路径指向的是文件而不是文件夹。请选择一个包含可运行工厂的本地工厂文件夹。",
    openSessionFolderNotRunnableError:
      "已找到此文件夹，但其中不包含可运行的工厂。请选择一个包含可运行工厂的文件夹后重试。",
    openSessionFolderUnreadableError:
      "无法从这台设备读取此文件夹。请检查权限，然后选择一个可读取的本地工厂文件夹。",
    openSessionFolderUnknownError:
      "仪表板无法验证此工厂文件夹。请检查路径后重试。",
    openSessionLaunchReadyMultipleTargets:
      "文件夹已就绪。请选择一个可运行目标后继续。",
    openSessionLaunchReadySingleTarget:
      "文件夹已就绪。请打开下方显示的可运行目标后继续。",
    openSessionDialogTitle: "打开工厂文件夹",
    openSessionSubmitLabel: "检查文件夹",
    openSessionSubmitPendingLabel: "正在检查文件夹...",
    pauseSessionStreamLabelTemplate: "暂停 {{sessionLabel}} 更新",
    openSessionValidationPendingLabel:
      "正在检查此文件夹是否包含可运行的工厂...",
    pauseSessionStreamLabelTemplate: "暂停 {{sessionLabel}} 更新",
    returnToCurrentTickLabel: "返回当前刻度",
    resumeSessionStreamLabelTemplate: "恢复 {{sessionLabel}} 更新",
    retrySessionsLabel: "重试会话",
    selectSessionTargetLabel: "打开会话目标",
    sessionFolderFieldLabel: "工厂文件夹",
    sessionFolderHelperText:
      "请输入本地工厂文件夹路径，或从这台设备选择一个文件夹。该文件夹必须包含可运行的工厂。",
    sessionFolderFieldPlaceholder: "/path/to/factory-workspace",
    sessionTabCloseLabelTemplate: "关闭 {{sessionLabel}} 会话",
    sessionTabsLabel: "工厂会话",
    sessionsEmptyTitle: "没有运行中的会话",
    sessionsErrorTitle: "工厂会话不可用",
    sessionsOfflineTitle: "工厂会话离线",
    sliderAriaLabel: "时间线刻度",
    sliderLabel: "时间线刻度",
    streamStatusConnectingLabel: "you-agent-factory 事件流正在连接",
    streamStatusLiveLabel: "you-agent-factory 事件流在线",
    streamStatusOfflineLabel: "you-agent-factory 事件流离线",
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
