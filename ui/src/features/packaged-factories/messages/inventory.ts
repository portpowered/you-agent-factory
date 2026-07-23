import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface PackagedFactoryInventoryMessages {
  readonly configurationCopied: string;
  readonly configurationCopyFailed: string;
  readonly configurationCopyLabel: (format: string) => string;
  readonly configurationFormatLabel: string;
  readonly configurationTitle: string;
  readonly catalogLabel: string;
  readonly catalogTitle: string;
  readonly descriptionUnavailable: string;
  readonly detailError: string;
  readonly detailLoading: string;
  readonly empty: string;
  readonly invalidContract: string;
  readonly inventoryLabel: string;
  readonly invocationCopied: string;
  readonly invocationCopyFailed: string;
  readonly invocationCopyLabel: (name: string) => string;
  readonly invocationExamplesTitle: string;
  readonly loading: string;
  readonly noExamples: string;
  readonly projectLabel: string;
  readonly selected: (stableName: string) => string;
  readonly unsupportedVersion: (version: string) => string;
}

const packagedFactoryInventoryMessagesByLocale = {
  en: {
    configurationCopied: "Configuration copied.",
    configurationCopyFailed: "Could not copy configuration. Try again.",
    configurationCopyLabel: (format) => `Copy ${format} configuration`,
    configurationFormatLabel: "Configuration format",
    configurationTitle: "Configuration",
    catalogLabel: "Packaged Factory catalog",
    catalogTitle: "Packaged Factories",
    descriptionUnavailable: "Description unavailable",
    detailError:
      "This Factory could not be loaded. Select another Factory to continue.",
    detailLoading: "Loading selected Factory…",
    empty: "No Packaged Factories are available.",
    invalidContract: "The Packaged Factory catalog is unavailable.",
    inventoryLabel: "Available Packaged Factories",
    invocationCopied: "Invocation example copied.",
    invocationCopyFailed: "Could not copy invocation example. Try again.",
    invocationCopyLabel: (name) => `Copy ${name} invocation example`,
    invocationExamplesTitle: "Invocation examples",
    loading: "Loading Packaged Factories…",
    noExamples: "No invocation examples are available.",
    projectLabel: "Project",
    selected: (stableName) => `${stableName} selected`,
    unsupportedVersion: (version) =>
      `This website does not support Packaged Factory catalog format ${version}.`,
  },
  "zh-CN": {
    configurationCopied: "已复制配置。",
    configurationCopyFailed: "无法复制配置。请重试。",
    configurationCopyLabel: (format) => `复制 ${format} 配置`,
    configurationFormatLabel: "配置格式",
    configurationTitle: "配置",
    catalogLabel: "打包工厂目录",
    catalogTitle: "打包工厂",
    descriptionUnavailable: "暂无说明",
    detailError: "无法加载此工厂。请选择其他工厂以继续。",
    detailLoading: "正在加载所选工厂…",
    empty: "目前没有可用的打包工厂。",
    invalidContract: "打包工厂目录当前不可用。",
    inventoryLabel: "可用的打包工厂",
    invocationCopied: "已复制调用示例。",
    invocationCopyFailed: "无法复制调用示例。请重试。",
    invocationCopyLabel: (name) => `复制 ${name} 调用示例`,
    invocationExamplesTitle: "调用示例",
    loading: "正在加载打包工厂…",
    noExamples: "暂无调用示例。",
    projectLabel: "项目",
    selected: (stableName) => `已选择 ${stableName}`,
    unsupportedVersion: (version) =>
      `此网站不支持打包工厂目录格式 ${version}。`,
  },
} satisfies LocalizedMessageCatalog<PackagedFactoryInventoryMessages>;

export function getPackagedFactoryInventoryMessages(
  locale?: string | null,
): PackagedFactoryInventoryMessages {
  return resolveLocalizedMessages(
    packagedFactoryInventoryMessagesByLocale,
    locale,
  );
}

export { packagedFactoryInventoryMessagesByLocale };
