import type { GraphSemanticIconKind } from "../../flowchart/graph-semantic-icon";
import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardFlowAxisLegendMessages {
  collapseLabel: string;
  collapseToggleLabel: (targetLabel: string) => string;
  edgeLabels: {
    activeFlow: string;
    failurePath: string;
  };
  expandToggleLabel: (targetLabel: string) => string;
  iconLabel: (label: string) => string;
  iconLabels: Record<GraphSemanticIconKind, string>;
  minimizedLabel: string;
  title: string;
}

const dashboardFlowAxisLegendMessagesByLocale = {
  en: {
    collapseLabel: "Collapse",
    collapseToggleLabel: (targetLabel) => `Collapse ${targetLabel}`,
    edgeLabels: {
      activeFlow: "Active flow",
      failurePath: "Failure path",
    },
    expandToggleLabel: (targetLabel) => `Expand ${targetLabel}`,
    iconLabel: (label) => `${label} legend icon`,
    iconLabels: {
      "active-work": "Active work",
      constraint: "Constraint",
      cron: "Cron workstation",
      exhaustion: "Exhaustion rule",
      failed: "Failed state",
      limit: "Limit",
      processing: "Processing",
      queue: "Queue",
      repeater: "Repeater workstation",
      resource: "Resource",
      terminal: "Terminal",
      workstation: "Standard workstation",
    },
    minimizedLabel: "Legend",
    title: "Graph legend",
  },
  ja: {
    collapseLabel: "閉じる",
    collapseToggleLabel: (targetLabel) => `${targetLabel} を閉じる`,
    edgeLabels: {
      activeFlow: "アクティブなフロー",
      failurePath: "失敗パス",
    },
    expandToggleLabel: (targetLabel) => `${targetLabel} を開く`,
    iconLabel: (label) => `${label} の凡例アイコン`,
    iconLabels: {
      "active-work": "進行中の作業",
      constraint: "制約",
      cron: "Cron ワークステーション",
      exhaustion: "枯渇ルール",
      failed: "失敗状態",
      limit: "上限",
      processing: "処理中",
      queue: "キュー",
      repeater: "リピーターワークステーション",
      resource: "リソース",
      terminal: "完了状態",
      workstation: "標準ワークステーション",
    },
    minimizedLabel: "凡例",
    title: "グラフの凡例",
  },
  ko: {
    collapseLabel: "접기",
    collapseToggleLabel: (targetLabel) => `${targetLabel} 접기`,
    edgeLabels: {
      activeFlow: "활성 흐름",
      failurePath: "실패 경로",
    },
    expandToggleLabel: (targetLabel) => `${targetLabel} 펼치기`,
    iconLabel: (label) => `${label} 범례 아이콘`,
    iconLabels: {
      "active-work": "활성 작업",
      constraint: "제약",
      cron: "Cron 워크스테이션",
      exhaustion: "소진 규칙",
      failed: "실패 상태",
      limit: "한도",
      processing: "처리 중",
      queue: "대기열",
      repeater: "반복 워크스테이션",
      resource: "리소스",
      terminal: "종료 상태",
      workstation: "표준 워크스테이션",
    },
    minimizedLabel: "범례",
    title: "그래프 범례",
  },
  zh: {
    collapseLabel: "收起",
    collapseToggleLabel: (targetLabel) => `收起${targetLabel}`,
    edgeLabels: {
      activeFlow: "活动流",
      failurePath: "失败路径",
    },
    expandToggleLabel: (targetLabel) => `展开${targetLabel}`,
    iconLabel: (label) => `${label}图例图标`,
    iconLabels: {
      "active-work": "活动工作",
      constraint: "约束",
      cron: "Cron 工作站",
      exhaustion: "耗尽规则",
      failed: "失败状态",
      limit: "限制",
      processing: "处理中",
      queue: "队列",
      repeater: "重复器工作站",
      resource: "资源",
      terminal: "终止状态",
      workstation: "标准工作站",
    },
    minimizedLabel: "图例",
    title: "图表图例",
  },
} satisfies LocalizedMessages<DashboardFlowAxisLegendMessages>;

export function getDashboardFlowAxisLegendMessages(
  locale?: string | null,
): DashboardFlowAxisLegendMessages {
  return resolveLocalizedMessages(dashboardFlowAxisLegendMessagesByLocale, locale);
}

export { dashboardFlowAxisLegendMessagesByLocale };
