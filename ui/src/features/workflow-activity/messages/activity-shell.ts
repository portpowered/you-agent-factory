import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkflowActivityShellMessages {
  emptyMessage: string;
  emptyTitle: string;
  selectExhaustionRuleLabel: (workstationTitle: string) => string;
  selectStateLabel: (placeLabel: string) => string;
  selectWorkerLabel: (workerName: string) => string;
  selectWorkstationLabel: (workstationTitle: string) => string;
  title: string;
  viewportLabel: string;
  widgetTitle: string;
}

const workflowActivityShellMessagesByLocale = {
  en: {
    emptyMessage: "The factory has not published any workstation graph yet.",
    emptyTitle: "No workflow topology loaded",
    selectExhaustionRuleLabel: (workstationTitle) =>
      `Select ${workstationTitle} exhaustion rule`,
    selectStateLabel: (placeLabel) => `Select ${placeLabel} state`,
    selectWorkerLabel: (workerName) => `Select ${workerName} worker`,
    selectWorkstationLabel: (workstationTitle) =>
      `Select ${workstationTitle} workstation`,
    title: "Current activity",
    viewportLabel: "Work graph viewport",
    widgetTitle: "Factory graph",
  },
  "zh-CN": {
    emptyMessage: "这个工厂还没有发布任何工作站图。",
    emptyTitle: "尚未加载工作流拓扑",
    selectExhaustionRuleLabel: (workstationTitle) =>
      `选择 ${workstationTitle} 枯竭规则`,
    selectStateLabel: (placeLabel) => `选择 ${placeLabel} 状态`,
    selectWorkerLabel: (workerName) => `选择 ${workerName} 工作者`,
    selectWorkstationLabel: (workstationTitle) =>
      `选择 ${workstationTitle} 工作站`,
    title: "当前活动",
    viewportLabel: "工作图视口",
    widgetTitle: "工厂图",
  },
} satisfies LocalizedMessageCatalog<WorkflowActivityShellMessages>;

export function getWorkflowActivityShellMessages(
  locale?: string | null,
): WorkflowActivityShellMessages {
  return resolveLocalizedMessages(
    workflowActivityShellMessagesByLocale,
    locale,
  );
}

export { workflowActivityShellMessagesByLocale };
