import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactoryPngExportMessages {
  imageDecodeFailed: string;
  metadataWriteFailed: string;
}

const factoryPngExportMessagesByLocale = {
  en: {
    imageDecodeFailed: "The selected image could not be decoded for PNG export.",
    metadataWriteFailed: "The exported PNG metadata could not be written.",
  },
  "zh-CN": {
    imageDecodeFailed: "无法解码所选图片以导出 PNG。",
    metadataWriteFailed: "无法写入导出的 PNG 元数据。",
  },
} satisfies LocalizedMessageCatalog<FactoryPngExportMessages>;

export function getFactoryPngExportMessages(
  locale?: string | null,
): FactoryPngExportMessages {
  return resolveLocalizedMessages(factoryPngExportMessagesByLocale, locale);
}

export { factoryPngExportMessagesByLocale };
