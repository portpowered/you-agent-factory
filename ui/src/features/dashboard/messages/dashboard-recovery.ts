import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardRecoveryMessages {
  logicalSessionUnresolvedDetail: string;
  logicalSessionUnresolvedTitle: string;
  recoveryFailedDetail: string;
  recoveryFailedRefreshLabel: string;
  recoveryFailedRetryLabel: string;
  recoveryFailedTitle: string;
  preflightRetryAction: string;
  sessionNotFoundDetailTemplate: string;
  sessionNotFoundTitle: string;
  unknownRecoveryDetail: string;
  unknownRecoveryTitle: string;
}

const dashboardRecoveryMessagesByLocale = {
  en: {
    recoveryFailedDetail:
      "The dashboard could not restore this session automatically after the event cursor expired. Retry the session stream or refresh the page.",
    recoveryFailedRefreshLabel: "Refresh page",
    recoveryFailedRetryLabel: "Retry session stream",
    recoveryFailedTitle: "Session replay needs attention",
    preflightRetryAction: "Retry clean replay",
    logicalSessionUnresolvedDetail:
      "The backend could not remap this tab to a live session for the saved logical target. Cached dashboard state was cleared. Reopen the default session or clear local state, then retry a clean replay.",
    logicalSessionUnresolvedTitle: "Session remap failed",
    sessionNotFoundDetailTemplate:
      'The backend could not resolve the live session for "{{sessionId}}". Cached dashboard state was cleared. Reopen the session, then retry a clean replay.',
    sessionNotFoundTitle: "Session recovery required",
    unknownRecoveryDetail:
      "The backend rejected the cached dashboard recovery state. Cached replay data was cleared. Retry a clean replay when the session is ready.",
    unknownRecoveryTitle: "Dashboard recovery blocked",
  },
  ja: {
    preflightRetryAction: "クリーンな再生を再試行",
    logicalSessionUnresolvedDetail:
      "バックエンドは保存された論理ターゲットのライブセッションへこのタブを再マップできませんでした。キャッシュされたダッシュボード状態は消去されました。デフォルトセッションを再度開くかローカル状態を消去してから、クリーンな再生を再試行してください。",
    logicalSessionUnresolvedTitle: "セッションの再マップに失敗しました",
    recoveryFailedDetail:
      "イベントカーソルの期限切れ後に、このセッションをダッシュボードで自動復元できませんでした。セッションストリームを再試行するか、ページを再読み込みしてください。",
    recoveryFailedRefreshLabel: "ページを再読み込み",
    recoveryFailedRetryLabel: "セッションストリームを再試行",
    recoveryFailedTitle: "セッションの再生を復旧できません",
    sessionNotFoundDetailTemplate:
      "バックエンドは「{{sessionId}}」のライブセッションを解決できませんでした。キャッシュされたダッシュボード状態は消去されました。セッションを再度開いてから、クリーンな再生を再試行してください。",
    sessionNotFoundTitle: "セッションの復旧が必要です",
    unknownRecoveryDetail:
      "バックエンドがキャッシュされたダッシュボード復旧状態を拒否しました。キャッシュされた再生データは消去されました。セッションの準備ができたら、クリーンな再生を再試行してください。",
    unknownRecoveryTitle: "ダッシュボードの復旧がブロックされました",
  },
  ko: {
    preflightRetryAction: "클린 리플레이 다시 시도",
    logicalSessionUnresolvedDetail:
      "백엔드가 저장된 논리 대상에 대한 라이브 세션으로 이 탭을 다시 매핑하지 못했습니다. 캐시된 대시보드 상태를 지웠습니다. 기본 세션을 다시 열거나 로컬 상태를 지운 뒤 클린 리플레이를 다시 시도하세요.",
    logicalSessionUnresolvedTitle: "세션 재매핑 실패",
    recoveryFailedDetail:
      "이벤트 커서가 만료된 뒤 이 세션을 대시보드에서 자동으로 복원하지 못했습니다. 세션 스트림을 다시 시도하거나 페이지를 새로고침하세요.",
    recoveryFailedRefreshLabel: "페이지 새로고침",
    recoveryFailedRetryLabel: "세션 스트림 다시 시도",
    recoveryFailedTitle: "세션 재생을 복구할 수 없음",
    sessionNotFoundDetailTemplate:
      '"{{sessionId}}"에 대한 라이브 세션을 백엔드가 확인하지 못했습니다. 캐시된 대시보드 상태를 지웠습니다. 세션을 다시 연 뒤 클린 리플레이를 다시 시도하세요.',
    sessionNotFoundTitle: "세션 복구 필요",
    unknownRecoveryDetail:
      "백엔드가 캐시된 대시보드 복구 상태를 거부했습니다. 캐시된 리플레이 데이터를 지웠습니다. 세션 준비가 끝나면 클린 리플레이를 다시 시도하세요.",
    unknownRecoveryTitle: "대시보드 복구 차단됨",
  },
  "zh-CN": {
    preflightRetryAction: "重试干净回放",
    logicalSessionUnresolvedDetail:
      "后端无法将此标签页重新映射到已保存逻辑目标的活动会话。缓存的仪表板状态已清除。请重新打开默认会话或清除本地状态，然后重试干净回放。",
    logicalSessionUnresolvedTitle: "会话重映射失败",
    recoveryFailedDetail:
      "事件游标过期后，仪表板无法自动恢复此会话。请重试会话事件流，或刷新页面。",
    recoveryFailedRefreshLabel: "刷新页面",
    recoveryFailedRetryLabel: "重试会话事件流",
    recoveryFailedTitle: "会话重放需要处理",
    sessionNotFoundDetailTemplate:
      "后端无法解析“{{sessionId}}”的活动会话。缓存的仪表板状态已清除。重新打开该会话后，再重试一次干净回放。",
    sessionNotFoundTitle: "需要恢复会话",
    unknownRecoveryDetail:
      "后端拒绝了缓存的仪表板恢复状态。缓存的回放数据已清除。会话准备好后，请重试干净回放。",
    unknownRecoveryTitle: "仪表板恢复被阻止",
  },
} satisfies LocalizedMessageCatalog<DashboardRecoveryMessages>;

export function getDashboardRecoveryMessages(
  locale?: string | null,
): DashboardRecoveryMessages {
  return resolveLocalizedMessages(dashboardRecoveryMessagesByLocale, locale);
}
