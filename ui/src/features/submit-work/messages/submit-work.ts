import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface SubmitWorkMessages {
  cardTitle: string;
  requestLabel: string;
  requestHint: string;
  requestNameLabel: string;
  requestNamePlaceholder: string;
  requestPlaceholder: string;
  selectWorkTypePlaceholder: string;
  submitAction: string;
  submittingAction: string;
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
    requestLabel: "Request",
    requestHint: "Optional. Leave this blank to submit an empty request.",
    requestNameLabel: "Request name",
    requestNamePlaceholder: "Add an optional label for this request.",
    requestPlaceholder:
      "Optional: describe what you want this request to accomplish.",
    selectWorkTypePlaceholder: "Select a work type",
    submitAction: "Submit work",
    submittingAction: "Submitting...",
    workTypeLabel: "Work type",
    statusMessages: {
      emptyGuidance:
        "Choose a work type to continue. Request details are optional.",
      errorFallback: "We couldn't submit your request. Try again in a moment.",
      noWorkTypes: "No work types are available to submit right now.",
      ready: "Ready to submit. Request details are optional.",
      requestOnly: "Ready to submit. Request details are optional.",
      submitting: "Sending your request...",
      success: (traceId) => `Your request was submitted. Trace ID: ${traceId}.`,
      workTypeOnly:
        "Choose a work type to continue. Request details are optional.",
    },
    validationMessages: {
      bothMissing: "Choose a work type before submitting.",
      fallback: "Fix the highlighted fields before submitting.",
      requestRequired: "Request details are optional.",
      workTypeRequired: "Choose a work type before submitting.",
    },
  },
  "zh-CN": {
    cardTitle: "提交工作",
    requestLabel: "请求",
    requestHint: "可选。留空也可以提交空请求。",
    requestNameLabel: "请求名称",
    requestNamePlaceholder: "为此请求添加一个可选标签。",
    requestPlaceholder: "可选：描述你希望这个请求完成什么。",
    selectWorkTypePlaceholder: "选择工作类型",
    submitAction: "提交工作",
    submittingAction: "正在提交...",
    workTypeLabel: "工作类型",
    statusMessages: {
      emptyGuidance: "先选择工作类型，然后即可继续。请求详情为可选。",
      errorFallback: "无法提交你的请求。请稍后再试。",
      noWorkTypes: "当前没有可用于提交的工作类型。",
      ready: "可以提交了。请求详情为可选。",
      requestOnly: "可以提交了。请求详情为可选。",
      submitting: "正在发送你的请求...",
      success: (traceId) => `你的请求已提交。追踪 ID：${traceId}。`,
      workTypeOnly: "先选择一个工作类型，然后即可继续。请求详情为可选。",
    },
    validationMessages: {
      bothMissing: "提交前请选择工作类型。",
      fallback: "提交前请先修正高亮字段。",
      requestRequired: "请求详情为可选。",
      workTypeRequired: "提交前请选择工作类型。",
    },
  },
} satisfies LocalizedMessageCatalog<SubmitWorkMessages>;

export function getSubmitWorkMessages(locale?: string | null): SubmitWorkMessages {
  return resolveLocalizedMessages(submitWorkMessagesByLocale, locale);
}

export { submitWorkMessagesByLocale };
