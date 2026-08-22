import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardSessionControlsMessages {
  factoryLifecyclePausedLabel: string;
  factoryLifecycleRunningLabel: string;
  liveDashboardUpdatesPausedLabel: string;
  pauseLiveUpdatesLabelTemplate: string;
  resumeLiveUpdatesLabelTemplate: string;
  timelineModeHistoricalLabel: string;
  timelineModeLabel: string;
  timelineModeLiveLabel: string;
}

const dashboardSessionControlsMessagesByLocale = {
  en: {
    factoryLifecyclePausedLabel: "Factory Session paused",
    factoryLifecycleRunningLabel: "Factory Session running",
    liveDashboardUpdatesPausedLabel: "Live dashboard updates paused",
    pauseLiveUpdatesLabelTemplate:
      "Pause live dashboard updates for {{sessionLabel}}",
    resumeLiveUpdatesLabelTemplate:
      "Resume live dashboard updates for {{sessionLabel}}",
    timelineModeHistoricalLabel: "Historical",
    timelineModeLabel: "Timeline mode",
    timelineModeLiveLabel: "Live",
  },
  ja: {
    factoryLifecyclePausedLabel: "ファクトリーセッションは一時停止中",
    factoryLifecycleRunningLabel: "ファクトリーセッションは実行中",
    liveDashboardUpdatesPausedLabel: "ライブダッシュボード更新を一時停止中",
    pauseLiveUpdatesLabelTemplate:
      "{{sessionLabel}} のライブダッシュボード更新を一時停止",
    resumeLiveUpdatesLabelTemplate:
      "{{sessionLabel}} のライブダッシュボード更新を再開",
    timelineModeHistoricalLabel: "履歴",
    timelineModeLabel: "タイムラインモード",
    timelineModeLiveLabel: "ライブ",
  },
  ko: {
    factoryLifecyclePausedLabel: "팩토리 세션 일시중지",
    factoryLifecycleRunningLabel: "팩토리 세션 실행 중",
    liveDashboardUpdatesPausedLabel: "라이브 대시보드 업데이트 일시중지",
    pauseLiveUpdatesLabelTemplate:
      "{{sessionLabel}} 라이브 대시보드 업데이트 일시중지",
    resumeLiveUpdatesLabelTemplate:
      "{{sessionLabel}} 라이브 대시보드 업데이트 다시 시작",
    timelineModeHistoricalLabel: "기록",
    timelineModeLabel: "타임라인 모드",
    timelineModeLiveLabel: "라이브",
  },
  "zh-CN": {
    factoryLifecyclePausedLabel: "工厂会话已暂停",
    factoryLifecycleRunningLabel: "工厂会话运行中",
    liveDashboardUpdatesPausedLabel: "实时仪表板更新已暂停",
    pauseLiveUpdatesLabelTemplate: "暂停 {{sessionLabel}} 的实时仪表板更新",
    resumeLiveUpdatesLabelTemplate: "恢复 {{sessionLabel}} 的实时仪表板更新",
    timelineModeHistoricalLabel: "历史",
    timelineModeLabel: "时间线模式",
    timelineModeLiveLabel: "实时",
  },
} satisfies LocalizedMessageCatalog<DashboardSessionControlsMessages>;

export function getDashboardSessionControlsMessages(
  locale?: string | null,
): DashboardSessionControlsMessages {
  return resolveLocalizedMessages(
    dashboardSessionControlsMessagesByLocale,
    locale,
  );
}

export { dashboardSessionControlsMessagesByLocale };
