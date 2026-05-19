import {
  formatNumber,
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkTotalsMessages {
  cardTitle: string;
  regionLabel: string;
  statLabels: {
    completed: string;
    dispatched: string;
    failed: string;
    inFlight: string;
  };
  statValueLabel: (label: string, value: number) => string;
}

const workTotalsMessagesByLocale = {
  en: {
    cardTitle: "Work totals",
    regionLabel: "work totals",
    statLabels: {
      completed: "Completed",
      dispatched: "Dispatched",
      failed: "Failed",
      inFlight: "In progress",
    },
    statValueLabel: (label, value) => `${label}: ${formatNumber(value, "en")}`,
  },
  "zh-CN": {
    cardTitle: "工作总计",
    regionLabel: "工作总计",
    statLabels: {
      completed: "已完成",
      dispatched: "已分派",
      failed: "失败",
      inFlight: "进行中",
    },
    statValueLabel: (label, value) => `${label}：${formatNumber(value, "zh-CN")}`,
  },
} satisfies LocalizedMessageCatalog<WorkTotalsMessages>;

export function getWorkTotalsMessages(locale?: string | null): WorkTotalsMessages {
  return resolveLocalizedMessages(workTotalsMessagesByLocale, locale);
}

export { workTotalsMessagesByLocale };
