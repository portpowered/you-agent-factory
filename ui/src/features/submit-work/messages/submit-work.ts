import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface SubmitWorkMessages {
  cardTitle: string;
  requestLabel: string;
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
    requestNameLabel: "Request name",
    requestNamePlaceholder: "Add an optional label for this request.",
    requestPlaceholder: "Describe what you want this request to accomplish.",
    selectWorkTypePlaceholder: "Select a work type",
    submitAction: "Submit work",
    submittingAction: "Submitting...",
    workTypeLabel: "Work type",
    statusMessages: {
      emptyGuidance: "Choose a work type and describe what you need to get started.",
      errorFallback: "We couldn't submit your request. Try again in a moment.",
      noWorkTypes: "No work types are available to submit right now.",
      ready: "Your request is ready to submit.",
      requestOnly: "Describe what you need to continue.",
      submitting: "Sending your request...",
      success: (traceId) => `Your request was submitted. Trace ID: ${traceId}.`,
      workTypeOnly: "Choose a work type to continue.",
    },
    validationMessages: {
      bothMissing: "Choose a work type and describe your request before submitting.",
      fallback: "Fix the highlighted fields before submitting.",
      requestRequired: "Describe your request before submitting.",
      workTypeRequired: "Choose a work type before submitting.",
    },
  },
  "zh-CN": {
    cardTitle: "提交工作",
    requestLabel: "请求",
    requestNameLabel: "请求名称",
    requestNamePlaceholder: "为此请求添加一个可选标签。",
    requestPlaceholder: "描述你希望这个请求完成什么。",
    selectWorkTypePlaceholder: "选择工作类型",
    submitAction: "提交工作",
    submittingAction: "正在提交...",
    workTypeLabel: "工作类型",
    statusMessages: {
      emptyGuidance: "先选择工作类型，再描述你需要完成什么。",
      errorFallback: "无法提交你的请求。请稍后再试。",
      noWorkTypes: "当前没有可用于提交的工作类型。",
      ready: "你的请求已准备好提交。",
      requestOnly: "描述你需要继续完成的内容。",
      submitting: "正在发送你的请求...",
      success: (traceId) => `你的请求已提交。追踪 ID：${traceId}。`,
      workTypeOnly: "选择一个工作类型以继续。",
    },
    validationMessages: {
      bothMissing: "提交前请选择工作类型并描述你的请求。",
      fallback: "提交前请先修正高亮字段。",
      requestRequired: "提交前请描述你的请求。",
      workTypeRequired: "提交前请选择工作类型。",
    },
  },
} satisfies LocalizedMessageCatalog<SubmitWorkMessages>;

export function getSubmitWorkMessages(locale?: string | null): SubmitWorkMessages {
  return resolveLocalizedMessages(submitWorkMessagesByLocale, locale);
}

export { submitWorkMessagesByLocale };
