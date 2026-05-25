import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface SubmitWorkMessages {
  addItemAction: string;
  addItemMenuLabel: string;
  addItemMenuDescription: string;
  addItemOptionLabel: (type: SubmitWorkItemTypeLabelKey) => string;
  cardTitle: string;
  chooseFileAction: string;
  fileItemPlaceholder: (typeLabel: string) => string;
  fileItemDragActive: (typeLabel: string) => string;
  fileItemFailure: (typeLabel: string) => string;
  fileItemInputLabel: (typeLabel: string) => string;
  fileItemMetadata: (fileName: string, mediaType: string) => string;
  fileItemReady: (fileName: string, mediaType: string) => string;
  fileItemStaging: (fileName: string) => string;
  removeItemLabel: (typeLabel: string, position: number) => string;
  replaceFileAction: string;
  requestHint?: string;
  requestItemLabel: (position: number) => string;
  requestNameLabel: string;
  requestNamePlaceholder: string;
  requestPlaceholder: string;
  selectWorkTypePlaceholder: string;
  submissionItemsLabel: string;
  submitAction: string;
  submittingAction: string;
  textItemTypeLabel: string;
  workTypeLabel: string;
  statusMessages: {
    emptyGuidance: string;
    errorFallback: string;
    noWorkTypes: string;
    ready: string;
    requestOnly: string;
    fileItemsNeedAttention: string;
    submitting: string;
    success: (traceId: string) => string;
    workTypeOnly: string;
  };
  validationMessages: {
    bothMissing: string;
    fileItemNeedsStaging: string;
    fileItemStillStaging: string;
    fallback: string;
    requestRequired: string;
    submissionItemsRequired: string;
    workTypeRequired: string;
  };
}

export type SubmitWorkItemTypeLabelKey =
  | "audio"
  | "document"
  | "image"
  | "text"
  | "video";

const submitWorkMessagesByLocale = {
  en: {
    addItemAction: "Add input",
    addItemMenuDescription: "Choose the next item to append to this submission.",
    addItemMenuLabel: "Add input menu",
    addItemOptionLabel: (type) =>
      ({
        audio: "Audio",
        document: "Document",
        image: "Image",
        text: "Text",
        video: "Video",
      })[type],
    cardTitle: "Submit work",
    chooseFileAction: "Choose file",
    fileItemDragActive: (typeLabel) =>
      `Drop the ${typeLabel.toLowerCase()} file to stage it.`,
    fileItemFailure: (typeLabel) =>
      `Retry staging this ${typeLabel.toLowerCase()} file or choose a different file.`,
    fileItemInputLabel: (typeLabel) => `${typeLabel} file`,
    fileItemMetadata: (fileName, mediaType) =>
      `${fileName} (${mediaType || "unknown type"})`,
    fileItemPlaceholder: (typeLabel) =>
      `Drop or choose one ${typeLabel.toLowerCase()} file to stage it for this submission.`,
    fileItemReady: (fileName, mediaType) =>
      `Staged ${fileName} (${mediaType || "unknown type"}).`,
    fileItemStaging: (fileName) => `Staging ${fileName}...`,
    removeItemLabel: (typeLabel, position) =>
      `Remove ${typeLabel.toLowerCase()} item ${position}`,
    replaceFileAction: "Replace file",
    requestNameLabel: "Request name",
    requestNamePlaceholder: "Add a name for this request.",
    requestPlaceholder:
      "Optional: describe what you want this request to accomplish.",
    requestItemLabel: (position) => `Text item ${position}`,
    selectWorkTypePlaceholder: "Select a work type",
    submissionItemsLabel: "Submission items",
    submitAction: "Submit work",
    submittingAction: "Submitting...",
    textItemTypeLabel: "Text",
    workTypeLabel: "Work type",
    statusMessages: {
      emptyGuidance:
        "Choose a work type and enter a request name to continue.",
      errorFallback: "We couldn't submit your request. Try again in a moment.",
      fileItemsNeedAttention:
        "Stage each file-backed item before submitting.",
      noWorkTypes: "No work types are available to submit right now.",
      ready: "Ready to submit.",
      requestOnly: "Enter a request name to continue.",
      submitting: "Sending your request...",
      success: (traceId) => `Your request was submitted. Trace ID: ${traceId}.`,
      workTypeOnly: "Choose a work type to continue.",
    },
    validationMessages: {
      bothMissing:
        "Choose a work type and enter a request name before submitting.",
      fileItemNeedsStaging:
        "Stage each file-backed item before submitting.",
      fileItemStillStaging:
        "Wait for file staging to finish before submitting.",
      fallback: "Fix the highlighted fields before submitting.",
      requestRequired: "Enter a request name before submitting.",
      submissionItemsRequired:
        "Add at least one non-empty text item or one staged file before submitting.",
      workTypeRequired: "Choose a work type before submitting.",
    },
  },
  "zh-CN": {
    addItemAction: "添加输入",
    addItemMenuDescription: "选择要追加到此提交中的下一项。",
    addItemMenuLabel: "添加输入菜单",
    addItemOptionLabel: (type) =>
      ({
        audio: "音频",
        document: "文档",
        image: "图像",
        text: "文本",
        video: "视频",
      })[type],
    cardTitle: "提交工作",
    chooseFileAction: "选择文件",
    fileItemDragActive: (typeLabel) => `拖放${typeLabel}文件以上传暂存。`,
    fileItemFailure: (typeLabel) =>
      `重新暂存这个${typeLabel}文件，或改选另一个文件。`,
    fileItemInputLabel: (typeLabel) => `${typeLabel}文件`,
    fileItemMetadata: (fileName, mediaType) =>
      `${fileName}（${mediaType || "未知类型"}）`,
    fileItemPlaceholder: (typeLabel) =>
      `拖放或选择一个${typeLabel}文件以暂存到此提交中。`,
    fileItemReady: (fileName, mediaType) =>
      `已暂存 ${fileName}（${mediaType || "未知类型"}）。`,
    fileItemStaging: (fileName) => `正在暂存 ${fileName}...`,
    removeItemLabel: (typeLabel, position) => `移除${typeLabel}项 ${position}`,
    replaceFileAction: "替换文件",
    requestNameLabel: "请求名称",
    requestNamePlaceholder: "为此请求添加名称。",
    requestPlaceholder: "可选：描述你希望这个请求完成什么。",
    requestItemLabel: (position) => `文本项 ${position}`,
    selectWorkTypePlaceholder: "选择工作类型",
    submissionItemsLabel: "提交项",
    submitAction: "提交工作",
    submittingAction: "正在提交...",
    textItemTypeLabel: "文本",
    workTypeLabel: "工作类型",
    statusMessages: {
      emptyGuidance: "先选择工作类型并填写请求名称，然后即可继续。",
      errorFallback: "无法提交你的请求。请稍后再试。",
      fileItemsNeedAttention: "提交前请先暂存每个文件项。",
      noWorkTypes: "当前没有可用于提交的工作类型。",
      ready: "可以提交了。",
      requestOnly: "请先填写请求名称。",
      submitting: "正在发送你的请求...",
      success: (traceId) => `你的请求已提交。追踪 ID：${traceId}。`,
      workTypeOnly: "先选择一个工作类型，然后即可继续。",
    },
    validationMessages: {
      bothMissing: "提交前请选择工作类型并填写请求名称。",
      fileItemNeedsStaging: "提交前请先暂存每个文件项。",
      fileItemStillStaging: "请等待文件暂存完成后再提交。",
      fallback: "提交前请先修正高亮字段。",
      requestRequired: "提交前请填写请求名称。",
      submissionItemsRequired: "提交前请至少添加一项非空文本或一个已暂存文件。",
      workTypeRequired: "提交前请选择工作类型。",
    },
  },
} satisfies LocalizedMessageCatalog<SubmitWorkMessages>;

export function getSubmitWorkMessages(locale?: string | null): SubmitWorkMessages {
  return resolveLocalizedMessages(submitWorkMessagesByLocale, locale);
}

export { submitWorkMessagesByLocale };
