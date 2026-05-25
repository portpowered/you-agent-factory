import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface SubmitWorkMessages {
  cardTitle: string;
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
    submitting: string;
    success: (traceId: string) => string;
    workTypeOnly: string;
  };
  validationMessages: {
    bothMissing: string;
    fallback: string;
    requestRequired: string;
    workTypeRequired: string;
  };
}

const submitWorkMessagesByLocale = {
  en: {
    cardTitle: "Submit work",
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
      fallback: "Fix the highlighted fields before submitting.",
      requestRequired: "Enter a request name before submitting.",
      workTypeRequired: "Choose a work type before submitting.",
    },
  },
  "zh-CN": {
    cardTitle: "提交工作",
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
      noWorkTypes: "当前没有可用于提交的工作类型。",
      ready: "可以提交了。",
      requestOnly: "请先填写请求名称。",
      submitting: "正在发送你的请求...",
      success: (traceId) => `你的请求已提交。追踪 ID：${traceId}。`,
      workTypeOnly: "先选择一个工作类型，然后即可继续。",
    },
    validationMessages: {
      bothMissing: "提交前请选择工作类型并填写请求名称。",
      fallback: "提交前请先修正高亮字段。",
      requestRequired: "提交前请填写请求名称。",
      workTypeRequired: "提交前请选择工作类型。",
    },
  },
} satisfies LocalizedMessageCatalog<SubmitWorkMessages>;

export function getSubmitWorkMessages(locale?: string | null): SubmitWorkMessages {
  return resolveLocalizedMessages(submitWorkMessagesByLocale, locale);
}

export { submitWorkMessagesByLocale };
