import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactoryPngImportMessages {
  factoryMetadataInvalidJson: string;
  factoryMetadataInvalidPayload: string;
  factoryMetadataMissing: string;
  factoryMetadataMissingName: string;
  factoryMetadataMissingSchemaVersion: string;
  factoryMetadataMustBeObject: string;
  fileReadFailed: string;
  imageDecodeFailed: string;
  notPngFile: string;
  pngInvalid: string;
  previewUnavailable: string;
  unsupportedSchemaVersion: string;
}

const factoryPngImportMessagesByLocale = {
  en: {
    factoryMetadataInvalidJson:
      "The you-agent-factory factory metadata is not valid JSON.",
    factoryMetadataInvalidPayload:
      "The you-agent-factory factory metadata does not contain a valid factory payload.",
    factoryMetadataMissing:
      "The selected PNG does not contain you-agent-factory factory metadata.",
    factoryMetadataMissingName:
      "The you-agent-factory factory metadata is missing the factory name.",
    factoryMetadataMissingSchemaVersion:
      "The you-agent-factory factory metadata is missing the schema version.",
    factoryMetadataMustBeObject:
      "The you-agent-factory factory metadata must be an object.",
    fileReadFailed: "The selected image could not be read.",
    imageDecodeFailed: "The selected image could not be decoded for preview.",
    notPngFile: "The selected file is not a PNG image.",
    pngInvalid: "The selected PNG image is invalid or truncated.",
    previewUnavailable:
      "The browser could not create a preview for the selected image.",
    unsupportedSchemaVersion:
      "The selected PNG uses an unsupported you-agent-factory factory metadata version.",
  },
  "zh-CN": {
    factoryMetadataInvalidJson: "you-agent-factory 工厂元数据不是有效的 JSON。",
    factoryMetadataInvalidPayload:
      "you-agent-factory 工厂元数据不包含有效的工厂负载。",
    factoryMetadataMissing: "所选 PNG 不包含 you-agent-factory 工厂元数据。",
    factoryMetadataMissingName: "you-agent-factory 工厂元数据缺少工厂名称。",
    factoryMetadataMissingSchemaVersion:
      "you-agent-factory 工厂元数据缺少架构版本。",
    factoryMetadataMustBeObject:
      "you-agent-factory 工厂元数据必须是一个对象。",
    fileReadFailed: "无法读取所选图片。",
    imageDecodeFailed: "无法解码所选图片以生成预览。",
    notPngFile: "所选文件不是 PNG 图片。",
    pngInvalid: "所选 PNG 图片无效或已截断。",
    previewUnavailable: "浏览器无法为所选图片创建预览。",
    unsupportedSchemaVersion:
      "所选 PNG 使用了不受支持的 you-agent-factory 工厂元数据版本。",
  },
} satisfies LocalizedMessageCatalog<FactoryPngImportMessages>;

export function getFactoryPngImportMessages(
  locale?: string | null,
): FactoryPngImportMessages {
  return resolveLocalizedMessages(factoryPngImportMessagesByLocale, locale);
}

export { factoryPngImportMessagesByLocale };
