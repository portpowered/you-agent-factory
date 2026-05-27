import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { GraphSemanticIconKind } from "../components/graph-semantic-icon";
import type { WorkstationSemanticKind } from "../lib/workstation-icon-metadata";

export interface ActivityGraphMessages {
  activeBadgeLabel: string;
  activeItemCountLabel: (count: number) => string;
  graphSemanticIconLabel: (kind: GraphSemanticIconKind) => string;
  placeKindLabel: (place: DashboardPlaceRef) => string;
  placeSemanticIconLabel: (place: DashboardPlaceRef) => string;
  tokenCountLabel: (place: DashboardPlaceRef, count: number) => string;
  unknownGraphSemanticIconLabel: string;
  workstationIconLabel: (kind: WorkstationSemanticKind) => string;
}

function englishPlaceKindLabel(place: DashboardPlaceRef): string {
  if (place.kind === "work_state") {
    if (place.state_category === "TERMINAL") {
      return "Terminal";
    }
    if (place.state_category === "FAILED") {
      return "Failed";
    }
    return "Queue";
  }

  if (place.kind === "resource") {
    return "Resource";
  }

  return place.kind === "limit" ? "Limit" : "Constraint";
}

function chinesePlaceKindLabel(place: DashboardPlaceRef): string {
  if (place.kind === "work_state") {
    if (place.state_category === "TERMINAL") {
      return "终止状态";
    }
    if (place.state_category === "FAILED") {
      return "失败状态";
    }
    return "队列";
  }

  if (place.kind === "resource") {
    return "资源";
  }

  return place.kind === "limit" ? "限制" : "约束";
}

function englishWorkstationIconLabel(kind: WorkstationSemanticKind): string {
  switch (kind) {
    case "cron":
      return "Cron workstation";
    case "poller":
      return "Poller workstation";
    case "exhaustion":
      return "Exhaustion rule";
    case "repeater":
      return "Repeater workstation";
    case "standard":
      return "Standard workstation";
  }
}

function chineseWorkstationIconLabel(kind: WorkstationSemanticKind): string {
  switch (kind) {
    case "cron":
      return "Cron 工作站";
    case "poller":
      return "轮询器工作站";
    case "exhaustion":
      return "耗尽规则";
    case "repeater":
      return "重复器工作站";
    case "standard":
      return "标准工作站";
  }
}

const activityGraphMessagesByLocale: LocalizedMessageCatalog<ActivityGraphMessages> = {
  en: {
    activeBadgeLabel: "Active",
    activeItemCountLabel: (count) =>
      `${count} active ${count === 1 ? "item" : "items"}`,
    graphSemanticIconLabel: (kind) => {
      switch (kind) {
        case "active-work":
          return "Active work";
        case "constraint":
          return "Constraint";
        case "cron":
          return "Cron workstation";
        case "exhaustion":
          return "Exhaustion rule";
        case "failed":
          return "Failed state";
        case "limit":
          return "Limit";
        case "poller":
          return "Poller workstation";
        case "processing":
          return "Processing state";
        case "queue":
          return "Queue state";
        case "repeater":
          return "Repeater workstation";
        case "resource":
          return "Resource";
        case "terminal":
          return "Terminal state";
        case "workstation":
          return "Workstation";
      }
    },
    placeKindLabel: englishPlaceKindLabel,
    placeSemanticIconLabel: (place) =>
      place.kind === "work_state" && place.state_category === "PROCESSING"
        ? "Processing state"
        : englishPlaceKindLabel(place),
    tokenCountLabel: (place, count) => {
      if (place.kind === "resource") {
        return `${count} resource tokens`;
      }

      const tokenLabel = count === 1 ? "token" : "tokens";
      return `${count} ${englishPlaceKindLabel(place).toLowerCase()} ${tokenLabel}`;
    },
    unknownGraphSemanticIconLabel: "Unknown graph semantic",
    workstationIconLabel: englishWorkstationIconLabel,
  },
  "zh-CN": {
    activeBadgeLabel: "活动",
    activeItemCountLabel: (count) => `${count} 个活动项`,
    graphSemanticIconLabel: (kind) => {
      switch (kind) {
        case "active-work":
          return "活动工作";
        case "constraint":
          return "约束";
        case "cron":
          return "Cron 工作站";
        case "exhaustion":
          return "耗尽规则";
        case "failed":
          return "失败状态";
        case "limit":
          return "限制";
        case "poller":
          return "轮询器工作站";
        case "processing":
          return "处理中状态";
        case "queue":
          return "队列状态";
        case "repeater":
          return "重复器工作站";
        case "resource":
          return "资源";
        case "terminal":
          return "终止状态";
        case "workstation":
          return "工作站";
      }
    },
    placeKindLabel: chinesePlaceKindLabel,
    placeSemanticIconLabel: (place) =>
      place.kind === "work_state" && place.state_category === "PROCESSING"
        ? "处理中状态"
        : chinesePlaceKindLabel(place),
    tokenCountLabel: (place, count) => {
      if (place.kind === "resource") {
        return `${count} 个资源令牌`;
      }

      return `${count} 个${chinesePlaceKindLabel(place)}令牌`;
    },
    unknownGraphSemanticIconLabel: "未知图语义",
    workstationIconLabel: chineseWorkstationIconLabel,
  },
};

export function getActivityGraphMessages(
  locale?: string | null,
): ActivityGraphMessages {
  return resolveLocalizedMessages(activityGraphMessagesByLocale, locale);
}

export { activityGraphMessagesByLocale };
