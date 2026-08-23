import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { GraphSemanticIconKind } from "../../flowchart/components/graph-semantic-icon";

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
      antigravity: "Antigravity runner",
      claude: "Claude runner",
      codex: "Codex runner",
      constraint: "Constraint",
      doc: "Doc",
      cron: "Cron workstation",
      failed: "Failed state",
      gemini: "Gemini runner",
      limit: "Limit",
      poller: "Poller workstation",
      processing: "Processing",
      queue: "Queue",
      repeater: "Repeater workstation",
      resource: "Resource",
      script: "Script worker",
      terminal: "Terminal",
      worker: "Worker",
      "work-type": "Work type",
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
      antigravity: "Antigravity ランナー",
      claude: "Claude ランナー",
      codex: "Codex ランナー",
      constraint: "制約",
      doc: "ドキュメント",
      cron: "Cron ワークステーション",
      failed: "失敗状態",
      gemini: "Gemini ランナー",
      limit: "上限",
      poller: "ポーラーワークステーション",
      processing: "処理中",
      queue: "キュー",
      repeater: "リピーターワークステーション",
      resource: "リソース",
      script: "スクリプトワーカー",
      terminal: "完了状態",
      worker: "ワーカー",
      "work-type": "作業タイプ",
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
      antigravity: "Antigravity 러너",
      claude: "Claude 러너",
      codex: "Codex 러너",
      constraint: "제약",
      doc: "문서",
      cron: "Cron 워크스테이션",
      failed: "실패 상태",
      gemini: "Gemini 러너",
      limit: "한도",
      poller: "폴러 워크스테이션",
      processing: "처리 중",
      queue: "대기열",
      repeater: "반복 워크스테이션",
      resource: "리소스",
      script: "스크립트 작업자",
      terminal: "종료 상태",
      worker: "작업자",
      "work-type": "작업 유형",
      workstation: "표준 워크스테이션",
    },
    minimizedLabel: "범례",
    title: "그래프 범례",
  },
  "zh-CN": {
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
      antigravity: "Antigravity 运行器",
      claude: "Claude 运行器",
      codex: "Codex 运行器",
      constraint: "约束",
      doc: "文档",
      cron: "Cron 工作站",
      failed: "失败状态",
      gemini: "Gemini 运行器",
      limit: "限制",
      poller: "轮询器工作站",
      processing: "处理中",
      queue: "队列",
      repeater: "重复器工作站",
      resource: "资源",
      script: "脚本工作者",
      terminal: "终止状态",
      worker: "工作者",
      "work-type": "工作类型",
      workstation: "标准工作站",
    },
    minimizedLabel: "图例",
    title: "图表图例",
  },
} satisfies LocalizedMessages<DashboardFlowAxisLegendMessages>;

export function getDashboardFlowAxisLegendMessages(
  locale?: string | null,
): DashboardFlowAxisLegendMessages {
  return resolveLocalizedMessages(
    dashboardFlowAxisLegendMessagesByLocale,
    locale,
  );
}

export { dashboardFlowAxisLegendMessagesByLocale };
