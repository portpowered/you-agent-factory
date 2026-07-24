import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { DocDetailMessages } from "./doc-detail-types";

const docDetailMessagesByLocale = {
  en: {
    configurationEmpty:
      "This doc is no longer attached to the current factory.",
    configurationErrorPrefix: "Unable to load the selected doc.",
    configurationLoading: "Loading doc details…",
    configurationUnknownError: "Unknown error",
    docKindLabel: "Factory doc",
    editableConfigurationCollapseActionLabel: "Collapse doc editor",
    editableConfigurationContractInvalidPrefix: "Doc configuration is invalid.",
    editableConfigurationEditorError: "The doc editor could not start.",
    editableConfigurationEditorLoading: "Loading the doc editor…",
    editableConfigurationEmpty:
      "This running factory definition does not expose editable doc values.",
    editableConfigurationErrorPrefix: "Doc configuration unavailable.",
    editableConfigurationExpandActionLabel: "Expand doc editor",
    editableConfigurationFileNameDuplicate: (fileName) =>
      `A doc named "${fileName}" already exists in the running factory definition.`,
    editableConfigurationFileNameDotSegments:
      "Doc paths cannot contain '.' or '..' segments.",
    editableConfigurationFileNameFieldLabel: "file name",
    editableConfigurationFileNameForwardSlashes:
      "Doc paths must use forward slashes.",
    editableConfigurationFileNameInvalid:
      "Enter a valid file name under factory/docs/.",
    editableConfigurationFileNameMustBeFile: "Doc paths must point to a file.",
    editableConfigurationFileNameOutsideDocsRoot:
      "Doc paths must stay under factory/docs/.",
    editableConfigurationFileNameRequired:
      "Enter a doc file name before saving.",
    editableConfigurationHeading: "Doc editor",
    editableConfigurationInlineContentFieldLabel: "doc content",
    editableConfigurationInlineContentRequired:
      "Enter doc content before saving.",
    editableConfigurationLoading: "Loading editable doc configuration.",
    editableConfigurationOverwriteWarning: (fields) =>
      `The running factory changed after you started editing. Saving now will overwrite newer server values for ${fields}.`,
    editableConfigurationOverwriteWarningDetail:
      "Review the latest runtime values before saving, or keep editing if this draft should replace them.",
    editableConfigurationSaveAction: "Save doc",
    editableConfigurationSaveBusyAction: "Saving doc...",
    editableConfigurationSaveDisabledValidationDetail:
      "Save stays disabled until the highlighted doc fields are valid.",
    editableConfigurationSaveErrorPrefix: "Saving failed.",
    editableConfigurationSaveFallbackError:
      "The running factory could not be saved.",
    editableConfigurationSaveStaleVersionDetail:
      "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
    editableConfigurationSaveSuccess: (displayLabel) =>
      `Running factory saved. ${displayLabel} was updated in the running factory definition.`,
    editableConfigurationServerFieldChangedHint:
      "The running factory changed this field while you were editing. Discard local changes to use the latest server-backed value.",
    editableConfigurationTargetPathPrefix: "Path",
    editableConfigurationValidationStatus:
      "Resolve the highlighted fields before saving this doc.",
    resetToLatestAction: "Discard local changes",
  },
  "zh-CN": {
    configurationEmpty: "该文档已不再附加到当前工厂。",
    configurationErrorPrefix: "无法加载所选文档。",
    configurationLoading: "正在加载文档详情…",
    configurationUnknownError: "未知错误",
    docKindLabel: "工厂文档",
    editableConfigurationCollapseActionLabel: "收起文档编辑器",
    editableConfigurationContractInvalidPrefix: "文档配置无效。",
    editableConfigurationEditorError: "文档编辑器无法启动。",
    editableConfigurationEditorLoading: "正在加载文档编辑器…",
    editableConfigurationEmpty: "当前运行中的工厂定义没有可编辑的文档值。",
    editableConfigurationErrorPrefix: "文档配置不可用。",
    editableConfigurationExpandActionLabel: "展开文档编辑器",
    editableConfigurationFileNameDuplicate: (fileName) =>
      `运行中的工厂定义中已存在名为“${fileName}”的文档。`,
    editableConfigurationFileNameDotSegments: "文档路径不能包含“.”或“..”段。",
    editableConfigurationFileNameFieldLabel: "文件名",
    editableConfigurationFileNameForwardSlashes: "文档路径必须使用正斜杠。",
    editableConfigurationFileNameInvalid:
      "请输入 factory/docs/ 下的有效文件名。",
    editableConfigurationFileNameMustBeFile: "文档路径必须指向文件。",
    editableConfigurationFileNameOutsideDocsRoot:
      "文档路径必须位于 factory/docs/ 下。",
    editableConfigurationFileNameRequired: "保存前请输入文档文件名。",
    editableConfigurationHeading: "文档编辑器",
    editableConfigurationInlineContentFieldLabel: "文档内容",
    editableConfigurationInlineContentRequired: "保存前请输入文档内容。",
    editableConfigurationLoading: "正在加载可编辑的文档配置。",
    editableConfigurationOverwriteWarning: (fields) =>
      `您开始编辑后，运行中的工厂已发生变化。现在保存将覆盖 ${fields} 的较新服务器值。`,
    editableConfigurationOverwriteWarningDetail:
      "保存前请查看最新运行时值，或继续编辑以用此草稿替换它们。",
    editableConfigurationSaveAction: "保存文档",
    editableConfigurationSaveBusyAction: "正在保存文档...",
    editableConfigurationSaveDisabledValidationDetail:
      "在突出显示的文档字段有效之前，保存将保持禁用。",
    editableConfigurationSaveErrorPrefix: "保存失败。",
    editableConfigurationSaveFallbackError: "无法保存运行中的工厂。",
    editableConfigurationSaveStaleVersionDetail:
      "重新加载最新的运行中工厂值，或保留此草稿并在编辑器刷新后重试。",
    editableConfigurationSaveSuccess: (displayLabel) =>
      `运行中的工厂已保存。${displayLabel} 已在运行中的工厂定义中更新。`,
    editableConfigurationServerFieldChangedHint:
      "您编辑期间，运行中的工厂更改了此字段。放弃本地更改以使用最新的服务器值。",
    editableConfigurationTargetPathPrefix: "路径",
    editableConfigurationValidationStatus: "保存此文档前请解决突出显示的字段。",
    resetToLatestAction: "放弃本地更改",
  },
} satisfies LocalizedMessageCatalog<DocDetailMessages>;

export function getDocDetailMessages(
  locale?: string | null,
): DocDetailMessages {
  return resolveLocalizedMessages(docDetailMessagesByLocale, locale);
}

export { docDetailMessagesByLocale };
