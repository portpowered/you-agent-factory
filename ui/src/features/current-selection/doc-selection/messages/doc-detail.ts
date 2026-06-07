import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../../i18n";

export interface DocDetailMessages {
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  configurationUnknownError: string;
  docKindLabel: string;
}

const docDetailMessagesByLocale = {
  en: {
    configurationEmpty: "This doc is no longer attached to the current factory.",
    configurationErrorPrefix: "Unable to load the selected doc.",
    configurationLoading: "Loading doc details…",
    configurationUnknownError: "Unknown error",
    docKindLabel: "Factory doc",
  },
  "zh-CN": {
    configurationEmpty: "该文档已不再附加到当前工厂。",
    configurationErrorPrefix: "无法加载所选文档。",
    configurationLoading: "正在加载文档详情…",
    configurationUnknownError: "未知错误",
    docKindLabel: "工厂文档",
  },
} satisfies LocalizedMessageCatalog<DocDetailMessages>;

export function getDocDetailMessages(
  locale?: string | null,
): DocDetailMessages {
  return resolveLocalizedMessages(docDetailMessagesByLocale, locale);
}

export { docDetailMessagesByLocale };
