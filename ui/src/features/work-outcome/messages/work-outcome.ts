import {
  formatNumber,
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkOutcomeMessages {
  chart: {
    ariaLabel: (rangeLabel: string) => string;
    cardRegionLabel: string;
    cardTitle: string;
    emptyMessage: string;
    emptyTitle: string;
    errorMessage: string;
    errorTitle: string;
    loadingMessage: string;
    loadingTitle: string;
    resetZoomAction: string;
    resetZoomLabel: string;
    seriesLabels: {
      completed: string;
      failed: string;
      inFlight: string;
      queued: string;
    };
    seriesPointLabel: (seriesLabel: string, value: number) => string;
    sessionRangeLabel: string;
    tickLabel: (tick: number) => string;
    workTypeFailureLabel: (workType: string) => string;
    xAxisLabel: string;
    yAxisLabel: string;
  };
  trends: {
    averageDurationLabel: string;
    causeGroupsEmpty: string;
    causeGroupsLabel: string;
    causeGroupsRegionLabel: string;
    failedInRangeLabel: string;
    failureChartAriaLabel: (rangeLabel: string) => string;
    failureEmptyMessage: string;
    failureEmptyTitle: string;
    failureSummary: string;
    failureTitle: string;
    fastestDurationLabel: string;
    latestDurationLabel: string;
    latestOutcomeLabel: string;
    rangeLabel: string;
    rangeOptionLabel: (rangeId: string, defaultLabel: string) => string;
    reworkPointLabel: (dispatchLabel: string, reworkCount: number) => string;
    reworkChartAriaLabel: (workLabel: string) => string;
    reworkEmptyMessage: string;
    reworkEmptyTitle: string;
    reworkSummary: string;
    reworkTitle: string;
    retryOrReworkLabel: string;
    slowestDurationLabel: string;
    timingChartAriaLabel: (workLabel: string) => string;
    timingEmptyMessage: string;
    timingRangeLabel: string;
    timingSummary: string;
    timingTitle: string;
    totalFailedLabel: string;
    traceWorkLabel: string;
  };
}

const workOutcomeMessagesByLocale = {
  en: {
    chart: {
      ariaLabel: (rangeLabel) => `Work outcome chart for ${rangeLabel}`,
      cardRegionLabel: "Work outcome chart region",
      cardTitle: "Work outcome chart",
      emptyMessage:
        "Work outcome data appears after the event stream receives work history.",
      emptyTitle: "No work outcome samples",
      errorMessage:
        "Chart data is incomplete, so the dashboard cannot draw this work outcome view yet.",
      errorTitle: "Work outcome chart unavailable",
      loadingMessage: "Waiting for dashboard timeline data.",
      loadingTitle: "Loading work outcome samples",
      resetZoomAction: "Reset zoom",
      resetZoomLabel: "Reset work outcome chart zoom",
      seriesLabels: {
        completed: "Completed",
        failed: "Failed/retried",
        inFlight: "In-flight",
        queued: "Queued",
      },
      seriesPointLabel: (seriesLabel, value) =>
        `${seriesLabel}: ${formatNumber(value, "en")}`,
      sessionRangeLabel: "Session",
      tickLabel: (tick) => `Tick ${formatNumber(tick, "en")}`,
      workTypeFailureLabel: (workType) => `Work type: ${workType}`,
      xAxisLabel: "Ticks",
      yAxisLabel: "Work count",
    },
    trends: {
      averageDurationLabel: "Average duration",
      causeGroupsEmpty: "No failed work has been grouped yet.",
      causeGroupsLabel: "Cause groups",
      causeGroupsRegionLabel: "Failure cause groups",
      failedInRangeLabel: "Failed in range",
      failureChartAriaLabel: (rangeLabel) => `Failed work trend for ${rangeLabel}`,
      failureEmptyMessage:
        "Failure trend data appears after the event stream receives work history.",
      failureEmptyTitle: "No failure samples",
      failureSummary: "Failed work and cause groups from the selected factory timeline.",
      failureTitle: "Failure trend",
      fastestDurationLabel: "Fastest",
      latestDurationLabel: "Latest",
      latestOutcomeLabel: "Latest outcome",
      rangeLabel: "Time range",
      rangeOptionLabel: (_rangeId, defaultLabel) => defaultLabel,
      reworkPointLabel: (dispatchLabel, reworkCount) =>
        `${dispatchLabel}: ${formatNumber(reworkCount, "en")} retry or rework events`,
      reworkChartAriaLabel: (workLabel) => `Retry and rework trend for ${workLabel}`,
      reworkEmptyMessage:
        "Select active work with retained trace history to see retry activity.",
      reworkEmptyTitle: "No selected trace",
      reworkSummary: "Reject, retry, or rework activity from the selected work trace.",
      reworkTitle: "Retry and rework trend",
      retryOrReworkLabel: "Retry or rework",
      slowestDurationLabel: "Slowest dispatch",
      timingChartAriaLabel: (workLabel) => `Timing trend for ${workLabel}`,
      timingEmptyMessage:
        "Select active work with retained trace history to compare dispatch timing.",
      timingRangeLabel: "Timing range",
      timingSummary: "Dispatch duration trend from the selected work trace.",
      timingTitle: "Timing trend",
      totalFailedLabel: "Total failed",
      traceWorkLabel: "Trace work",
    },
  },
  "zh-CN": {
    chart: {
      ariaLabel: (rangeLabel) => `${rangeLabel} 的工作结果图表`,
      cardRegionLabel: "工作结果图表区域",
      cardTitle: "工作结果图表",
      emptyMessage: "事件流接收到工作历史后，会显示工作结果数据。",
      emptyTitle: "暂无工作结果样本",
      errorMessage: "图表数据不完整，因此仪表板暂时无法绘制这个工作结果视图。",
      errorTitle: "工作结果图表不可用",
      loadingMessage: "正在等待仪表板时间线数据。",
      loadingTitle: "正在加载工作结果样本",
      resetZoomAction: "重置缩放",
      resetZoomLabel: "重置工作结果图表缩放",
      seriesLabels: {
        completed: "已完成",
        failed: "失败/重试",
        inFlight: "进行中",
        queued: "排队中",
      },
      seriesPointLabel: (seriesLabel, value) =>
        `${seriesLabel}：${formatNumber(value, "zh-CN")}`,
      sessionRangeLabel: "会话",
      tickLabel: (tick) => `刻度 ${formatNumber(tick, "zh-CN")}`,
      workTypeFailureLabel: (workType) => `工作类型：${workType}`,
      xAxisLabel: "刻度",
      yAxisLabel: "工作计数",
    },
    trends: {
      averageDurationLabel: "平均时长",
      causeGroupsEmpty: "尚未对失败工作进行分组。",
      causeGroupsLabel: "原因分组",
      causeGroupsRegionLabel: "失败原因分组",
      failedInRangeLabel: "范围内失败",
      failureChartAriaLabel: (rangeLabel) => `${rangeLabel} 的失败工作趋势`,
      failureEmptyMessage: "事件流接收到工作历史后，会显示失败趋势数据。",
      failureEmptyTitle: "暂无失败样本",
      failureSummary: "来自所选工厂时间线的失败工作和原因分组。",
      failureTitle: "失败趋势",
      fastestDurationLabel: "最快",
      latestDurationLabel: "最新",
      latestOutcomeLabel: "最新结果",
      rangeLabel: "时间范围",
      rangeOptionLabel: (rangeId, defaultLabel) =>
        rangeId === "session" ? "会话" : defaultLabel,
      reworkPointLabel: (dispatchLabel, reworkCount) =>
        `${dispatchLabel}：${formatNumber(reworkCount, "zh-CN")} 个重试或返工事件`,
      reworkChartAriaLabel: (workLabel) => `${workLabel} 的重试与返工趋势`,
      reworkEmptyMessage: "选择包含保留追踪历史的活动工作，以查看重试活动。",
      reworkEmptyTitle: "未选择追踪",
      reworkSummary: "来自所选工作追踪的拒绝、重试或返工活动。",
      reworkTitle: "重试与返工趋势",
      retryOrReworkLabel: "重试或返工",
      slowestDurationLabel: "最慢分派",
      timingChartAriaLabel: (workLabel) => `${workLabel} 的时序趋势`,
      timingEmptyMessage: "选择包含保留追踪历史的活动工作，以比较分派耗时。",
      timingRangeLabel: "时序范围",
      timingSummary: "来自所选工作追踪的分派耗时趋势。",
      timingTitle: "时序趋势",
      totalFailedLabel: "失败总数",
      traceWorkLabel: "追踪工作",
    },
  },
} satisfies LocalizedMessageCatalog<WorkOutcomeMessages>;

export function getWorkOutcomeMessages(locale?: string | null): WorkOutcomeMessages {
  return resolveLocalizedMessages(workOutcomeMessagesByLocale, locale);
}

export { workOutcomeMessagesByLocale };
