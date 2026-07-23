import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface PackagedFactoryInventoryMessages {
  readonly catalogLabel: string;
  readonly catalogTitle: string;
  readonly descriptionUnavailable: string;
  readonly detailError: string;
  readonly detailLoading: string;
  readonly empty: string;
  readonly invalidContract: string;
  readonly inventoryLabel: string;
  readonly loading: string;
  readonly projectLabel: string;
  readonly selected: (stableName: string) => string;
  readonly unsupportedVersion: (version: string) => string;
}

const packagedFactoryInventoryMessagesByLocale = {
  en: {
    catalogLabel: "Packaged Factory catalog",
    catalogTitle: "Packaged Factories",
    descriptionUnavailable: "Description unavailable",
    detailError:
      "This Factory could not be loaded. Select another Factory to continue.",
    detailLoading: "Loading selected Factory…",
    empty: "No Packaged Factories are available.",
    invalidContract: "The Packaged Factory catalog is unavailable.",
    inventoryLabel: "Available Packaged Factories",
    loading: "Loading Packaged Factories…",
    projectLabel: "Project",
    selected: (stableName) => `${stableName} selected`,
    unsupportedVersion: (version) =>
      `This website does not support Packaged Factory catalog format ${version}.`,
  },
  "zh-CN": {
    catalogLabel: "打包工厂目录",
    catalogTitle: "打包工厂",
    descriptionUnavailable: "暂无说明",
    detailError: "无法加载此工厂。请选择其他工厂以继续。",
    detailLoading: "正在加载所选工厂…",
    empty: "目前没有可用的打包工厂。",
    invalidContract: "打包工厂目录当前不可用。",
    inventoryLabel: "可用的打包工厂",
    loading: "正在加载打包工厂…",
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
