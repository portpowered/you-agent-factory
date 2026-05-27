import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import {
  DASHBOARD_WIDGET_PICKER_WIDGET_TYPES,
  type DashboardWidgetPickerWidgetType,
} from "../lib/dashboard-widget-picker";

interface InlineWidgetPickerOptionMessages {
  description: string;
  title: string;
}

export interface InlineWidgetPickerMessages {
  addedState: string;
  closeLabel: string;
  description: string;
  dismissAction: string;
  duplicateBadge: string;
  enabledState: string;
  openAction: string;
  options: Record<
    DashboardWidgetPickerWidgetType,
    InlineWidgetPickerOptionMessages
  >;
  selectionHint: string;
  title: string;
}

const inlineWidgetPickerMessagesByLocale = {
  en: {
    addedState: "Already on dashboard",
    closeLabel: "Close widget picker",
    description:
      "Choose from the dashboard widgets available for inline management.",
    dismissAction: "Close",
    duplicateBadge: "Duplicate allowed",
    enabledState: "Add widget",
    openAction: "Browse widgets",
    options: {
      "current-selection": {
        description:
          "Keep the active work, node, and trace context visible while you explore the factory.",
        title: "Current selection",
      },
      "provider-session": {
        description:
          "Follow the selected provider session without leaving the dashboard grid.",
        title: "Provider session",
      },
      "submit-work": {
        description:
          "Create new work requests directly from the dashboard surface.",
        title: "Submit work",
      },
      "terminal-work": {
        description:
          "Review finished and failed work items in one compact list.",
        title: "Terminal work",
      },
      trace: {
        description:
          "Inspect trace details and related state transitions inline.",
        title: "Trace drilldown",
      },
      "work-graph": {
        description:
          "Watch workflow activity and navigate the graph from the main dashboard board.",
        title: "Workflow activity",
      },
      "work-outcome-chart": {
        description: "Compare recent completion and failure trends over time.",
        title: "Work outcome chart",
      },
      "work-totals": {
        description:
          "Track headline counts for dispatched, completed, and failed work.",
        title: "Work totals",
      },
    },
    selectionHint: "Choose a widget to add it immediately to the grid.",
    title: "Add dashboard widget",
  },
  "zh-CN": {
    addedState: "已在仪表板中",
    closeLabel: "关闭小组件选择器",
    description: "从当前可用于内联管理的仪表板小组件中进行选择。",
    dismissAction: "关闭",
    duplicateBadge: "允许重复",
    enabledState: "添加小组件",
    openAction: "浏览小组件",
    options: {
      "current-selection": {
        description: "在浏览工厂时持续显示当前工作、节点和追踪上下文。",
        title: "当前选择",
      },
      "provider-session": {
        description: "无需离开仪表板网格即可跟踪所选提供方会话。",
        title: "提供方会话",
      },
      "submit-work": {
        description: "直接从仪表板界面创建新的工作请求。",
        title: "提交工作",
      },
      "terminal-work": {
        description: "在一个紧凑列表中查看已完成和失败的工作项。",
        title: "终端工作",
      },
      trace: {
        description: "内联检查追踪详情及其相关状态变化。",
        title: "追踪钻取",
      },
      "work-graph": {
        description: "从主仪表板面板观察工作流活动并导航流程图。",
        title: "工作流活动",
      },
      "work-outcome-chart": {
        description: "比较最近一段时间的完成与失败趋势。",
        title: "工作结果图表",
      },
      "work-totals": {
        description: "跟踪已分派、已完成和失败工作的关键计数。",
        title: "工作总览",
      },
    },
    selectionHint: "选择一个小组件即可立即将其加入网格。",
    title: "添加仪表板小组件",
  },
} satisfies LocalizedMessageCatalog<InlineWidgetPickerMessages>;

export function getInlineWidgetPickerMessages(
  locale?: string | null,
): InlineWidgetPickerMessages {
  return resolveLocalizedMessages(inlineWidgetPickerMessagesByLocale, locale);
}

export function getInlineWidgetPickerOptions(locale?: string | null): Array<{
  description: string;
  title: string;
  widgetType: DashboardWidgetPickerWidgetType;
}> {
  const messages = getInlineWidgetPickerMessages(locale);

  return DASHBOARD_WIDGET_PICKER_WIDGET_TYPES.map((widgetType) => ({
    description: messages.options[widgetType].description,
    title: messages.options[widgetType].title,
    widgetType,
  }));
}

export { inlineWidgetPickerMessagesByLocale };
