import {
  type LocalizedMessageCatalog,
  localizeEnumLabel,
  resolveLocalizedMessages,
} from "../../../../i18n";

export interface ResourceDetailEnumMessages {
  localizeResourceType: (value: string) => string;
}

const resourceDetailEnumMessagesByLocale = {
  en: {
    localizeResourceType: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          INVOCATION_SLOT: "Invocation slot",
          MODEL: "Model",
          PROVIDER_QUOTA: "Provider quota",
        },
        locale: "en",
        value,
      }),
  },
  ja: {
    localizeResourceType: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          INVOCATION_SLOT: "呼び出しスロット",
          MODEL: "モデル",
          PROVIDER_QUOTA: "プロバイダー割当",
        },
        locale: "ja",
        value,
      }),
  },
  ko: {
    localizeResourceType: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          INVOCATION_SLOT: "호출 슬롯",
          MODEL: "모델",
          PROVIDER_QUOTA: "프로바이더 할당량",
        },
        locale: "ko",
        value,
      }),
  },
  "zh-CN": {
    localizeResourceType: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          INVOCATION_SLOT: "调用槽位",
          MODEL: "模型",
          PROVIDER_QUOTA: "提供商配额",
        },
        locale: "zh-CN",
        value,
      }),
  },
} satisfies LocalizedMessageCatalog<ResourceDetailEnumMessages>;

export function getResourceDetailEnumMessages(
  locale?: string | null,
): ResourceDetailEnumMessages {
  return resolveLocalizedMessages(resourceDetailEnumMessagesByLocale, locale);
}
