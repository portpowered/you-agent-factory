import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkflowActivityShellMessages {
  emptyMessage: string;
  emptyTitle: string;
  selectDocLabel: (docName: string) => string;
  selectStateLabel: (placeLabel: string) => string;
  selectResourceLabel: (resourceName: string) => string;
  selectWorkerLabel: (workerName: string) => string;
  selectWorkTypeLabel: (workTypeName: string) => string;
  selectWorkstationLabel: (workstationTitle: string) => string;
  title: string;
  viewportLabel: string;
  widgetTitle: string;
}

const workflowActivityShellMessagesByLocale = {
  en: {
    emptyMessage: "The factory has not published any workstation graph yet.",
    emptyTitle: "No workflow topology loaded",
    selectDocLabel: (docName) => `Select ${docName} doc`,
    selectStateLabel: (placeLabel) => `Select ${placeLabel} state`,
    selectResourceLabel: (resourceName) => `Select ${resourceName} resource`,
    selectWorkerLabel: (workerName) => `Select ${workerName} worker`,
    selectWorkTypeLabel: (workTypeName) => `Select ${workTypeName} work type`,
    selectWorkstationLabel: (workstationTitle) =>
      `Select ${workstationTitle} workstation`,
    title: "Current activity",
    viewportLabel: "Work graph viewport",
    widgetTitle: "Factory graph",
  },
  "zh-CN": {
    emptyMessage: "这个工厂还没有发布任何工作站图。",
    emptyTitle: "尚未加载工作流拓扑",
    selectDocLabel: (docName) => `选择 ${docName} 文档`,
    selectStateLabel: (placeLabel) => `选择 ${placeLabel} 状态`,
    selectResourceLabel: (resourceName) => `选择 ${resourceName} 资源`,
    selectWorkerLabel: (workerName) => `选择 ${workerName} 工作者`,
    selectWorkTypeLabel: (workTypeName) => `选择 ${workTypeName} 工作类型`,
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
