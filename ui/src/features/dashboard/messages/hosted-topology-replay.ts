import type {
  FactoryTimelineScrubberMessages,
  FactoryTopologyReplayMessages,
} from "@you-agent-factory/factory-visualizers";

import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface HostedTopologyReplayMessages {
  formatTick: (tick: number) => string;
  stream: {
    offline: string;
    reconnecting: string;
    recoveryFailed: string;
  };
  timeline: FactoryTimelineScrubberMessages;
  topology: FactoryTopologyReplayMessages;
}

const hostedTopologyReplayMessagesByLocale = {
  en: {
    formatTick: (tick) => `Tick ${tick}`,
    stream: {
      offline:
        "Factory updates are unavailable. Showing the last received state.",
      reconnecting:
        "Reconnecting to Factory updates. Showing the last received state.",
      recoveryFailed:
        "Factory replay recovery needs attention. Showing the last restored state.",
    },
    timeline: {
      alreadyFollowingLatest: "Already following the latest Factory state.",
      currentMode: "Following live Factory updates.",
      disabled: "Replay controls are temporarily unavailable.",
      followLatest: "Follow latest",
      historyMode: "Viewing historical Factory state.",
      position: (selected, latest) => `${selected} of ${latest}`,
      regionLabel: "Factory replay timeline",
      sliderLabel: "Select Factory replay tick",
      title: "Replay",
      unavailable: "Factory replay is not available yet.",
    },
    topology: {
      activeDispatches: (count) =>
        count === 1 ? "1 active Dispatch" : `${count} active Dispatches`,
      empty: "No Factory topology is available at this tick.",
      failed: "The Factory topology could not be displayed.",
      inactiveDispatches: "No active Dispatch",
      loading: "Loading Factory topology...",
      nodeLabel: (kind, label) => `Select ${label} ${kind}`,
      regionLabel: "Factory topology",
      resourceOccupancy: (occupied, capacity) =>
        `${occupied} of ${capacity} resource units occupied`,
      resourceOccupancyUnavailable: "Resource occupancy unavailable",
      retry: "Retry topology",
      selectedNode: "Selected",
      workStateCount: (count) => (count === 1 ? "1 Work" : `${count} Work`),
      workStateCountUnavailable: "Work count unavailable",
    },
  },
  "zh-CN": {
    formatTick: (tick) => `时刻 ${tick}`,
    stream: {
      offline: "工厂更新暂时不可用。正在显示最后收到的状态。",
      reconnecting: "正在重新连接工厂更新。正在显示最后收到的状态。",
      recoveryFailed: "工厂重放恢复需要处理。正在显示最后恢复的状态。",
    },
    timeline: {
      alreadyFollowingLatest: "已在跟随最新工厂状态。",
      currentMode: "正在跟随实时工厂更新。",
      disabled: "重放控件暂时不可用。",
      followLatest: "跟随最新状态",
      historyMode: "正在查看工厂历史状态。",
      position: (selected, latest) => `${selected} / ${latest}`,
      regionLabel: "工厂重放时间线",
      sliderLabel: "选择工厂重放时刻",
      title: "重放",
      unavailable: "工厂重放尚不可用。",
    },
    topology: {
      activeDispatches: (count) => `${count} 个活动派遣`,
      empty: "此时刻没有可用的工厂拓扑。",
      failed: "无法显示工厂拓扑。",
      inactiveDispatches: "没有活动派遣",
      loading: "正在加载工厂拓扑...",
      nodeLabel: (kind, label) => `选择 ${label} ${kind}`,
      regionLabel: "工厂拓扑",
      resourceOccupancy: (occupied, capacity) =>
        `${capacity} 个资源单位中已占用 ${occupied} 个`,
      resourceOccupancyUnavailable: "资源占用情况不可用",
      retry: "重试拓扑",
      selectedNode: "已选择",
      workStateCount: (count) => `${count} 个工作`,
      workStateCountUnavailable: "工作计数不可用",
    },
  },
} satisfies LocalizedMessageCatalog<HostedTopologyReplayMessages>;

export function getHostedTopologyReplayMessages(
  locale?: string | null,
): HostedTopologyReplayMessages {
  return resolveLocalizedMessages(hostedTopologyReplayMessagesByLocale, locale);
}

export { hostedTopologyReplayMessagesByLocale };
