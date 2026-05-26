import {
  localizeEnumLabel,
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkstationDetailEnumMessages {
  localizeWorkstationBehavior: (value: string) => string;
  localizeWorkstationKind: (value: string) => string;
}

const workstationDetailEnumMessagesByLocale = {
  en: {
    localizeWorkstationBehavior: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          CRON: "Cron",
          POLLER: "Poller",
          REPEATER: "Repeater",
          STANDARD: "Standard",
        },
        locale: "en",
        value,
      }),
    localizeWorkstationKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          cron: "Cron",
          poller: "Poller",
          repeater: "Repeater",
          standard: "Standard",
        },
        locale: "en",
        value,
      }),
  },
  ja: {
    localizeWorkstationBehavior: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          CRON: "Cron",
          POLLER: "ポーラー",
          REPEATER: "リピーター",
          STANDARD: "標準",
        },
        locale: "ja",
        value,
      }),
    localizeWorkstationKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          cron: "Cron",
          poller: "ポーラー",
          repeater: "リピーター",
          standard: "標準",
        },
        locale: "ja",
        value,
      }),
  },
  ko: {
    localizeWorkstationBehavior: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          CRON: "Cron",
          POLLER: "폴러",
          REPEATER: "반복기",
          STANDARD: "표준",
        },
        locale: "ko",
        value,
      }),
    localizeWorkstationKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          cron: "Cron",
          poller: "폴러",
          repeater: "반복기",
          standard: "표준",
        },
        locale: "ko",
        value,
      }),
  },
  "zh-CN": {
    localizeWorkstationBehavior: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          CRON: "Cron",
          POLLER: "轮询器",
          REPEATER: "重复器",
          STANDARD: "标准",
        },
        locale: "zh-CN",
        value,
      }),
    localizeWorkstationKind: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          cron: "Cron",
          poller: "轮询器",
          repeater: "重复器",
          standard: "标准",
        },
        locale: "zh-CN",
        value,
      }),
  },
} satisfies LocalizedMessageCatalog<WorkstationDetailEnumMessages>;

export function getWorkstationDetailEnumMessages(
  locale?: string | null,
): WorkstationDetailEnumMessages {
  return resolveLocalizedMessages(workstationDetailEnumMessagesByLocale, locale);
}
