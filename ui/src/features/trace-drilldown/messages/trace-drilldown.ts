import {
  type LocalizedMessageCatalog,
  localizeEnumLabel,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface TraceDrilldownMessages {
  batchRelationGraphLabel: string;
  batchRelationsLabel: string;
  dispatchColumnLabel: string;
  dispatchCountLabel: string;
  dispatchFlowLabel: string;
  dispatchPathEmpty: string;
  dispatchPathGraphLabel: string;
  dispatchPathInputPrefix: string;
  dispatchPathOutputPrefix: string;
  dispatchPathPendingOutcome: string;
  dispatchPathSectionLabel: string;
  emptyMessage: string;
  emptyTitle: string;
  errorTitle: string;
  idleMessage: string;
  idleTitle: string;
  inputItemsColumnLabel: string;
  loadingMessage: (workID: string) => string;
  loadingTitle: string;
  noBatchRelations: string;
  noInputItems: string;
  noOutputItems: string;
  noTraceHistoryMessage: string;
  noTraceHistoryTitle: string;
  unknownRelationSource: string;
  localizeRelationState: (value: string) => string;
  localizeRelationType: (value: string) => string;
  relationEdgeLabel: (params: {
    relationState?: string;
    relationType: string;
    sourceLabel: string;
    targetLabel: string;
  }) => string;
  requestIdsLabel: string;
  tableCaption: string;
  title: string;
  traceIdLabel: string;
  unknownWorkstationLabel: string;
  unavailableValue: string;
  workItemsExpandLabel: (expanded: boolean) => string;
  workItemsLabel: string;
  workItemsSummary: (count: number) => string;
  workstationColumnLabel: string;
  outcomeColumnLabel: string;
  outputItemsColumnLabel: string;
}

const traceDrilldownMessagesByLocale = {
  en: {
    batchRelationGraphLabel: "Batch relation graph",
    batchRelationsLabel: "Batch relations",
    dispatchColumnLabel: "Dispatch",
    dispatchCountLabel: "Dispatch count",
    dispatchFlowLabel: "Dispatch flow",
    dispatchPathEmpty: "Unavailable",
    dispatchPathGraphLabel: "Dispatch relationship graph",
    dispatchPathInputPrefix: "In",
    dispatchPathOutputPrefix: "Out",
    dispatchPathPendingOutcome: "Observed",
    dispatchPathSectionLabel: "Dispatch",
    emptyMessage:
      "No retained dispatch history is currently available for this work item.",
    emptyTitle: "Trace history unavailable",
    errorTitle: "Trace lookup failed",
    idleMessage:
      "Select active, completed, or failed work to inspect retained trace history.",
    idleTitle: "No trace selected",
    inputItemsColumnLabel: "Input items",
    loadingMessage: (workID) =>
      `Reconstructing dispatch history for ${workID}.`,
    loadingTitle: "Loading trace",
    noBatchRelations: "None",
    noInputItems: "No input items recorded.",
    noOutputItems: "No output items recorded.",
    noTraceHistoryMessage:
      "No retained dispatch history is currently available for this work item.",
    noTraceHistoryTitle: "Trace history unavailable",
    unknownRelationSource: "Unknown source",
    localizeRelationState: (value: string) =>
      localizeEnumLabel({
        category: "status",
        labels: {
          ACCEPTED: "Accepted",
          APPROVED: "Approved",
          COMPLETED: "Completed",
          DONE: "Done",
          FAILED: "Failed",
          READY: "Ready",
          REJECTED: "Rejected",
        },
        locale: "en",
        value,
      }),
    localizeRelationType: (value: string) =>
      localizeEnumLabel({
        category: "relation",
        labels: {
          DEPENDS_ON: "Depends on",
          PARENT_CHILD: "Parent-child",
          RELATED_TO: "Related to",
          RETRY: "Retry",
          SPAWNED_BY: "Spawned by",
        },
        locale: "en",
        value,
      }),
    relationEdgeLabel: ({
      relationState,
      relationType,
      sourceLabel,
      targetLabel,
    }) =>
      relationState
        ? `${localizeEnumLabel({
            category: "relation",
            labels: {
              DEPENDS_ON: "Depends on",
              PARENT_CHILD: "Parent-child",
              RELATED_TO: "Related to",
              RETRY: "Retry",
              SPAWNED_BY: "Spawned by",
            },
            locale: "en",
            value: relationType,
          })} relation from ${sourceLabel} to ${targetLabel}, requiring ${localizeEnumLabel(
            {
              category: "status",
              labels: {
                ACCEPTED: "Accepted",
                APPROVED: "Approved",
                COMPLETED: "Completed",
                DONE: "Done",
                FAILED: "Failed",
                READY: "Ready",
                REJECTED: "Rejected",
              },
              locale: "en",
              value: relationState,
            },
          )}`
        : `${localizeEnumLabel({
            category: "relation",
            labels: {
              DEPENDS_ON: "Depends on",
              PARENT_CHILD: "Parent-child",
              RELATED_TO: "Related to",
              RETRY: "Retry",
              SPAWNED_BY: "Spawned by",
            },
            locale: "en",
            value: relationType,
          })} relation from ${sourceLabel} to ${targetLabel}`,
    requestIdsLabel: "Request IDs",
    tableCaption: "Trace dispatch grid",
    title: "Trace drill-down",
    traceIdLabel: "Trace ID",
    unknownWorkstationLabel: "Unknown workstation",
    unavailableValue: "Unavailable",
    workItemsExpandLabel: (expanded): string =>
      expanded ? "Collapse" : "Expand",
    workItemsLabel: "Work items",
    workItemsSummary: (count) => `${count} work item${count === 1 ? "" : "s"}`,
    workstationColumnLabel: "Workstation",
    outcomeColumnLabel: "Outcome",
    outputItemsColumnLabel: "Output items",
  },
  "zh-CN": {
    batchRelationGraphLabel: "批次关系图",
    batchRelationsLabel: "批次关系",
    dispatchColumnLabel: "分派",
    dispatchCountLabel: "分派数量",
    dispatchFlowLabel: "分派流",
    dispatchPathEmpty: "不可用",
    dispatchPathGraphLabel: "分派关系图",
    dispatchPathInputPrefix: "输入",
    dispatchPathOutputPrefix: "输出",
    dispatchPathPendingOutcome: "已观测",
    dispatchPathSectionLabel: "分派",
    emptyMessage: "当前这个工作项暂时没有可保留的分派历史。",
    emptyTitle: "追踪历史不可用",
    errorTitle: "追踪查询失败",
    idleMessage: "选择活动、已完成或失败的工作，以查看保留的追踪历史。",
    idleTitle: "未选择追踪",
    inputItemsColumnLabel: "输入项",
    loadingMessage: (workID) => `正在为 ${workID} 重建分派历史。`,
    loadingTitle: "正在加载追踪",
    noBatchRelations: "无",
    noInputItems: "没有记录输入项。",
    noOutputItems: "没有记录输出项。",
    noTraceHistoryMessage: "当前这个工作项暂时没有可保留的分派历史。",
    noTraceHistoryTitle: "追踪历史不可用",
    unknownRelationSource: "未知来源",
    localizeRelationState: (value: string) =>
      localizeEnumLabel({
        category: "status",
        labels: {
          ACCEPTED: "已接受",
          APPROVED: "已批准",
          COMPLETED: "已完成",
          DONE: "已完成",
          FAILED: "失败",
          READY: "就绪",
          REJECTED: "已拒绝",
        },
        locale: "zh-CN",
        value,
      }),
    localizeRelationType: (value: string) =>
      localizeEnumLabel({
        category: "relation",
        labels: {
          DEPENDS_ON: "依赖项",
          PARENT_CHILD: "父子",
          RELATED_TO: "相关",
          RETRY: "重试",
          SPAWNED_BY: "派生自",
        },
        locale: "zh-CN",
        value,
      }),
    relationEdgeLabel: ({
      relationState,
      relationType,
      sourceLabel,
      targetLabel,
    }) =>
      relationState
        ? `${localizeEnumLabel({
            category: "relation",
            labels: {
              DEPENDS_ON: "依赖项",
              PARENT_CHILD: "父子",
              RELATED_TO: "相关",
              RETRY: "重试",
              SPAWNED_BY: "派生自",
            },
            locale: "zh-CN",
            value: relationType,
          })}关系：从 ${sourceLabel} 到 ${targetLabel}，要求 ${localizeEnumLabel(
            {
              category: "status",
              labels: {
                ACCEPTED: "已接受",
                APPROVED: "已批准",
                COMPLETED: "已完成",
                DONE: "已完成",
                FAILED: "失败",
                READY: "就绪",
                REJECTED: "已拒绝",
              },
              locale: "zh-CN",
              value: relationState,
            },
          )}`
        : `${localizeEnumLabel({
            category: "relation",
            labels: {
              DEPENDS_ON: "依赖项",
              PARENT_CHILD: "父子",
              RELATED_TO: "相关",
              RETRY: "重试",
              SPAWNED_BY: "派生自",
            },
            locale: "zh-CN",
            value: relationType,
          })}关系：从 ${sourceLabel} 到 ${targetLabel}`,
    requestIdsLabel: "请求 ID",
    tableCaption: "追踪分派表",
    title: "追踪下钻",
    traceIdLabel: "追踪 ID",
    unknownWorkstationLabel: "未知工作站",
    unavailableValue: "不可用",
    workItemsExpandLabel: (expanded): string => (expanded ? "折叠" : "展开"),
    workItemsLabel: "工作项",
    workItemsSummary: (count) => `${count} 个工作项`,
    workstationColumnLabel: "工作站",
    outcomeColumnLabel: "结果",
    outputItemsColumnLabel: "输出项",
  },
} satisfies LocalizedMessageCatalog<TraceDrilldownMessages>;

export function getTraceDrilldownMessages(
  locale?: string | null,
): TraceDrilldownMessages {
  return resolveLocalizedMessages(traceDrilldownMessagesByLocale, locale);
}

export { traceDrilldownMessagesByLocale };
