import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface DashboardWidgetRemovalMessages {
  cancelAction: string;
  closeDialog: string;
  confirmAction: string;
  confirmDescription: (widgetTitle: string) => string;
  confirmTitle: (widgetTitle: string) => string;
  dismissAction: string;
  failedToPersist: (widgetTitle: string) => string;
  removedWithStorageWarning: (widgetTitle: string) => string;
  removed: (widgetTitle: string) => string;
  restored: (widgetTitle: string) => string;
  undoAction: (widgetTitle: string) => string;
  undoLabel: string;
  undoDismissed: string;
  undoExpired: (widgetTitle: string) => string;
}

const dashboardWidgetRemovalMessagesByLocale = {
  en: {
    cancelAction: "Keep widget",
    closeDialog: "Close dialog",
    confirmAction: "Remove widget",
    confirmDescription: (widgetTitle: string) =>
      `${widgetTitle} has unsaved changes. Removing it will discard those changes.`,
    confirmTitle: (widgetTitle: string) => `Remove ${widgetTitle} widget?`,
    dismissAction: "Dismiss",
    failedToPersist: (widgetTitle: string) =>
      `${widgetTitle} was restored here, but the layout could not be saved.`,
    removedWithStorageWarning: (widgetTitle: string) =>
      `${widgetTitle} was removed here, but the layout could not be saved.`,
    removed: (widgetTitle: string) => `${widgetTitle} was removed.`,
    restored: (widgetTitle: string) => `${widgetTitle} was restored.`,
    undoAction: (widgetTitle: string) => `Undo removing ${widgetTitle}`,
    undoDismissed: "Undo dismissed.",
    undoExpired: (widgetTitle: string) =>
      `Undo expired for the removed ${widgetTitle} widget.`,
    undoLabel: "Undo",
  },
  "zh-CN": {
    cancelAction: "保留小组件",
    closeDialog: "关闭对话框",
    confirmAction: "移除小组件",
    confirmDescription: (widgetTitle: string) =>
      `${widgetTitle} 有未保存的更改。移除它将丢弃这些更改。`,
    confirmTitle: (widgetTitle: string) => `移除 ${widgetTitle} 小组件？`,
    dismissAction: "关闭提示",
    failedToPersist: (widgetTitle: string) =>
      `${widgetTitle} 已在此恢复，但布局无法保存。`,
    removedWithStorageWarning: (widgetTitle: string) =>
      `${widgetTitle} 已在此移除，但布局无法保存。`,
    removed: (widgetTitle: string) => `${widgetTitle} 已移除。`,
    restored: (widgetTitle: string) => `${widgetTitle} 已恢复。`,
    undoAction: (widgetTitle: string) => `撤销移除 ${widgetTitle}`,
    undoDismissed: "已关闭撤销提示。",
    undoExpired: (widgetTitle: string) =>
      `已移除的 ${widgetTitle} 小组件无法再撤销。`,
    undoLabel: "撤销",
  },
} satisfies LocalizedMessageCatalog<DashboardWidgetRemovalMessages>;

export function getDashboardWidgetRemovalMessages(
  locale?: string | null,
): DashboardWidgetRemovalMessages {
  return resolveLocalizedMessages(
    dashboardWidgetRemovalMessagesByLocale,
    locale,
  );
}

export { dashboardWidgetRemovalMessagesByLocale };
