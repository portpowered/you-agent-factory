import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { CliRelationship } from "../lib/cli-manifest-types";

export interface CliCommandExplorerMessages {
  commandNavigation: string;
  controls: string;
  emptyDescription: string;
  emptyTitle: string;
  examples: string;
  inheritedInput: string;
  inputs: (count: number) => string;
  invalidDescription: string;
  invalidTitle: string;
  lifecycle: (state: string) => string;
  loadingDescription: string;
  loadingTitle: string;
  localInput: string;
  noExamples: string;
  noInputs: string;
  noRelationships: string;
  relationship: (kind: CliRelationship["kind"], inputs: string) => string;
  relationships: string;
  selectedCommand: (path: string) => string;
  unsupportedControlDescription: (inputId: string, valueType: string) => string;
  unsupportedControlTitle: string;
  unsupportedDescription: (received: string, supported: string) => string;
  unsupportedTitle: string;
  usage: string;
  visibility: (visibility: string) => string;
}

const cliCommandExplorerMessagesByLocale = {
  en: {
    commandNavigation: "Commands",
    controls: "Static inputs",
    emptyDescription: "The published CLI manifest contains no commands.",
    emptyTitle: "No commands available",
    examples: "Examples",
    inheritedInput: "Inherited",
    inputs: (count) => `${count} ${count === 1 ? "input" : "inputs"}`,
    invalidDescription:
      "The published CLI contract could not be validated. Correct these diagnostics before displaying command details.",
    invalidTitle: "Invalid CLI contract",
    lifecycle: (state) => `Lifecycle: ${state}`,
    loadingDescription: "Validating the published CLI command manifest.",
    loadingTitle: "Loading CLI commands",
    localInput: "Local",
    noExamples: "No examples are published for this command.",
    noInputs: "This command has no configurable static inputs.",
    noRelationships: "This command has no input relationships.",
    relationship: (kind, inputs) => `${kind}: ${inputs}`,
    relationships: "Input relationships",
    selectedCommand: (path) => `Selected command: ${path}`,
    unsupportedControlDescription: (inputId, valueType) =>
      `Input ${inputId} uses unsupported type ${valueType}.`,
    unsupportedControlTitle: "Unsupported command input",
    unsupportedDescription: (received, supported) =>
      `Manifest version ${received} is not supported. Supported versions: ${supported}.`,
    unsupportedTitle: "Unsupported CLI manifest version",
    usage: "Usage",
    visibility: (visibility) => `Visibility: ${visibility}`,
  },
  "zh-CN": {
    commandNavigation: "命令",
    controls: "静态输入",
    emptyDescription: "已发布的 CLI 清单中没有命令。",
    emptyTitle: "没有可用命令",
    examples: "示例",
    inheritedInput: "继承",
    inputs: (count) => `${count} 个输入`,
    invalidDescription:
      "已发布的 CLI 契约无法通过校验。请先修正以下诊断，再显示命令详情。",
    invalidTitle: "CLI 契约无效",
    lifecycle: (state) => `生命周期：${state}`,
    loadingDescription: "正在校验已发布的 CLI 命令清单。",
    loadingTitle: "正在加载 CLI 命令",
    localInput: "本地",
    noExamples: "此命令没有已发布的示例。",
    noInputs: "此命令没有可配置的静态输入。",
    noRelationships: "此命令没有输入关系。",
    relationship: (kind, inputs) => `${kind}：${inputs}`,
    relationships: "输入关系",
    selectedCommand: (path) => `已选择命令：${path}`,
    unsupportedControlDescription: (inputId, valueType) =>
      `输入 ${inputId} 使用了不支持的类型 ${valueType}。`,
    unsupportedControlTitle: "不支持的命令输入",
    unsupportedDescription: (received, supported) =>
      `不支持清单版本 ${received}。支持的版本：${supported}。`,
    unsupportedTitle: "不支持的 CLI 清单版本",
    usage: "用法",
    visibility: (visibility) => `可见性：${visibility}`,
  },
} satisfies LocalizedMessageCatalog<CliCommandExplorerMessages>;

export function getCliCommandExplorerMessages(locale?: string | null) {
  return resolveLocalizedMessages(cliCommandExplorerMessagesByLocale, locale);
}
