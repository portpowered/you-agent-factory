import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface HeaderControlsMessages {
  currentTickStatusTemplate: string;
  dashboardSummaryLabel: string;
  returnToCurrentTickLabel: string;
  sliderAriaLabel: string;
  sliderLabel: string;
  streamStatusConnectingLabel: string;
  streamStatusLiveLabel: string;
  streamStatusOfflineLabel: string;
  waitingForMoreTicks: string;
}

export const HEADER_CURRENT_TICK_TOKEN = "{{currentTick}}";
export const HEADER_MAX_TICK_TOKEN = "{{maxTick}}";

const headerControlsMessagesByLocale = {
  en: {
    currentTickStatusTemplate:
      `Tick ${HEADER_CURRENT_TICK_TOKEN} of ${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "dashboard summary",
    returnToCurrentTickLabel: "Return to current tick",
    sliderAriaLabel: "Timeline tick",
    sliderLabel: "Timeline tick",
    streamStatusConnectingLabel: "Infinite You event stream connecting",
    streamStatusLiveLabel: "Infinite You event stream live",
    streamStatusOfflineLabel: "Infinite You event stream offline",
    waitingForMoreTicks: "Waiting for more ticks",
  },
  ja: {
    currentTickStatusTemplate:
      `${HEADER_MAX_TICK_TOKEN} 件中 ${HEADER_CURRENT_TICK_TOKEN} 件目のティック`,
    dashboardSummaryLabel: "ダッシュボードの概要",
    returnToCurrentTickLabel: "現在のティックに戻る",
    sliderAriaLabel: "タイムラインティック",
    sliderLabel: "タイムラインティック",
    streamStatusConnectingLabel: "Infinite You のイベントストリームを接続中",
    streamStatusLiveLabel: "Infinite You のイベントストリームはライブです",
    streamStatusOfflineLabel: "Infinite You のイベントストリームはオフラインです",
    waitingForMoreTicks: "ティックが増えるまで待機しています",
  },
  ko: {
    currentTickStatusTemplate:
      `틱 ${HEADER_CURRENT_TICK_TOKEN} / ${HEADER_MAX_TICK_TOKEN}`,
    dashboardSummaryLabel: "대시보드 요약",
    returnToCurrentTickLabel: "현재 틱으로 돌아가기",
    sliderAriaLabel: "타임라인 틱",
    sliderLabel: "타임라인 틱",
    streamStatusConnectingLabel: "Infinite You 이벤트 스트림에 연결 중",
    streamStatusLiveLabel: "Infinite You 이벤트 스트림이 라이브 상태입니다",
    streamStatusOfflineLabel: "Infinite You 이벤트 스트림이 오프라인 상태입니다",
    waitingForMoreTicks: "틱이 더 쌓일 때까지 기다리는 중",
  },
  zh: {
    currentTickStatusTemplate:
      `第 ${HEADER_CURRENT_TICK_TOKEN} 个刻度，共 ${HEADER_MAX_TICK_TOKEN} 个`,
    dashboardSummaryLabel: "仪表板概览",
    returnToCurrentTickLabel: "返回当前刻度",
    sliderAriaLabel: "时间线刻度",
    sliderLabel: "时间线刻度",
    streamStatusConnectingLabel: "Infinite You 事件流正在连接",
    streamStatusLiveLabel: "Infinite You 事件流在线",
    streamStatusOfflineLabel: "Infinite You 事件流离线",
    waitingForMoreTicks: "正在等待更多刻度",
  },
} satisfies LocalizedMessages<HeaderControlsMessages>;

export function getHeaderControlsMessages(
  locale?: string | null,
): HeaderControlsMessages {
  return resolveLocalizedMessages(headerControlsMessagesByLocale, locale);
}

export { headerControlsMessagesByLocale };
