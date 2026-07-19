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
  requestNameRequiredAffordance: string;
  requestNamePlaceholder: string;
  requestPlaceholder: string;
  selectWorkTypePlaceholder: string;
  simpleComposer: {
    errorFallback: string;
    formLabel: string;
    placeholder: string;
    submitAction: string;
    submittingAction: string;
    textLabel: string;
    unavailable: {
      "ambiguous-default": string;
      closed: string;
      error: string;
      history: string;
      invalid: string;
      loading: string;
      "no-default": string;
    };
  };
  submissionItemsLabel: string;
  submitAction: string;
  submittingAction: string;
  textItemTypeLabel: string;
  workTypeLabel: string;
  workTypeRequiredAffordance: string;
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
  invocation: {
    addRepeatedValue: (label: string) => string;
    aliases: (aliases: string[]) => string;
    booleanFalseAction: string;
    booleanTrueAction: string;
    booleanUnsetAction: string;
    cardTitle: string;
    defaultValue: (values: string[]) => string;
    directoryPathPlaceholder: string;
    emptyParametersState: string;
    exampleStdin: (value: string) => string;
    examplesTitle: string;
    filePathPlaceholder: string;
    loadingState: string;
    namedBinding: (name: string) => string;
    outputContentType: (value: string) => string;
    outputFileExtension: (value: string) => string;
    outputHintTitle: string;
    outputModeLabel: (mode: string) => string;
    outputPathParameter: (name: string) => string;
    pathPlaceholder: string;
    positionalBinding: (position: number) => string;
    primaryResultReady: string;
    removeRepeatedValue: (label: string, position: number) => string;
    requiredAffordance: string;
    selectOptionPlaceholder: string;
    statusMessages: {
      errorFallback: string;
      runtimeFailed: (status: string) => string;
      submitting: string;
      success: (traceId: string) => string;
      validationFailed: string;
    };
    stdinBinding: string;
    submitAction: string;
    submittingAction: string;
    textPlaceholder: string;
    validationMessages: {
      repeatedItemRequired: string;
      requiredField: (label: string) => string;
    };
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
    addItemMenuDescription:
      "Choose the next item to append to this submission.",
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
    requestNameRequiredAffordance: "required",
    requestNamePlaceholder: "Add a name for this request.",
    requestPlaceholder:
      "Optional: describe what you want this request to accomplish.",
    requestItemLabel: (position) => `Text item ${position}`,
    selectWorkTypePlaceholder: "Select a work type",
    simpleComposer: {
      errorFallback: "We couldn't submit this work. Try again.",
      formLabel: "Simple work submission",
      placeholder: "Describe the work to submit.",
      submitAction: "Submit",
      submittingAction: "Submitting...",
      textLabel: "Submit text",
      unavailable: {
        "ambiguous-default":
          "Multiple default work types are configured, so a submission cannot be routed safely.",
        closed: "This Factory is closed and cannot accept submissions.",
        error: "This Factory has an error and cannot accept submissions.",
        history: "Return to the latest Factory state to submit work.",
        invalid: "This Factory is invalid and cannot accept submissions.",
        loading: "The Factory is still loading. Try again when it is ready.",
        "no-default":
          "No eligible default work type is available for text submissions.",
      },
    },
    submissionItemsLabel: "Submission items",
    submitAction: "Submit work",
    submittingAction: "Submitting...",
    textItemTypeLabel: "Text",
    workTypeLabel: "Work type",
    workTypeRequiredAffordance: "required",
    statusMessages: {
      emptyGuidance: "Choose a work type and enter a request name to continue.",
      errorFallback: "We couldn't submit your request. Try again in a moment.",
      fileItemsNeedAttention: "Stage each file-backed item before submitting.",
      noWorkTypes: "No work types are available to submit right now.",
      ready: "Ready to submit.",
      requestOnly: "Enter a request name to continue.",
      submitting: "Sending your request...",
      success: (traceId) => `Your request was submitted. Trace ID: ${traceId}.`,
      workTypeOnly: "Choose a work type to continue.",
    },
    invocation: {
      addRepeatedValue: (label) => `Add ${label}`,
      aliases: (aliases) => `Aliases: ${aliases.join(", ")}`,
      booleanFalseAction: "False",
      booleanTrueAction: "True",
      booleanUnsetAction: "Use default",
      cardTitle: "Run factory",
      defaultValue: (values) =>
        `Default: ${values.length === 1 ? values[0] : values.join(", ")}`,
      directoryPathPlaceholder: "Enter a directory path.",
      emptyParametersState:
        "This factory can be invoked without additional arguments.",
      exampleStdin: (value) => `stdin: ${value}`,
      examplesTitle: "Examples",
      filePathPlaceholder: "Enter a file path.",
      loadingState: "Loading the current factory invocation contract...",
      namedBinding: (name) => `Named argument: --${name}`,
      outputContentType: (value) => `Content type: ${value}`,
      outputFileExtension: (value) => `File extension: ${value}`,
      outputHintTitle: "Output hint",
      outputModeLabel: (mode) => `Output mode: ${mode}`,
      outputPathParameter: (name) => `Output path argument: ${name}`,
      pathPlaceholder: "Enter a path.",
      positionalBinding: (position) => `Positional argument ${position}`,
      primaryResultReady: "Primary result",
      removeRepeatedValue: (label, position) =>
        `Remove ${label} value ${position}`,
      requiredAffordance: "required",
      selectOptionPlaceholder: "Select a value",
      statusMessages: {
        errorFallback:
          "We couldn't invoke this factory. Try again in a moment.",
        runtimeFailed: (status) => `Invocation finished with status ${status}.`,
        submitting: "Invoking the current factory...",
        success: (traceId) =>
          `Factory invocation started. Trace ID: ${traceId}.`,
        validationFailed: "Fix the highlighted arguments before invoking.",
      },
      stdinBinding: "Accepts stdin input.",
      submitAction: "Run factory",
      submittingAction: "Running...",
      textPlaceholder: "Enter a value.",
      validationMessages: {
        repeatedItemRequired: "Each repeated value must be non-empty.",
        requiredField: (label) => `Enter ${label} before invoking.`,
      },
    },
    validationMessages: {
      bothMissing:
        "Choose a work type and enter a request name before submitting.",
      fileItemNeedsStaging: "Stage each file-backed item before submitting.",
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
    requestNameRequiredAffordance: "必填",
    requestNamePlaceholder: "为此请求添加名称。",
    requestPlaceholder: "可选：描述你希望这个请求完成什么。",
    requestItemLabel: (position) => `文本项 ${position}`,
    selectWorkTypePlaceholder: "选择工作类型",
    simpleComposer: {
      errorFallback: "无法提交此工作。请重试。",
      formLabel: "简单工作提交",
      placeholder: "描述要提交的工作。",
      submitAction: "提交",
      submittingAction: "正在提交...",
      textLabel: "提交文本",
      unavailable: {
        "ambiguous-default": "配置了多个默认工作类型，因此无法安全路由提交。",
        closed: "此工厂已关闭，无法接受提交。",
        error: "此工厂发生错误，无法接受提交。",
        history: "请返回最新工厂状态以提交工作。",
        invalid: "此工厂无效，无法接受提交。",
        loading: "工厂仍在加载。准备就绪后请重试。",
        "no-default": "没有可用于文本提交的合格默认工作类型。",
      },
    },
    submissionItemsLabel: "提交项",
    submitAction: "提交工作",
    submittingAction: "正在提交...",
    textItemTypeLabel: "文本",
    workTypeLabel: "工作类型",
    workTypeRequiredAffordance: "必填",
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
    invocation: {
      addRepeatedValue: (label) => `添加${label}`,
      aliases: (aliases) => `别名：${aliases.join("、")}`,
      booleanFalseAction: "否",
      booleanTrueAction: "是",
      booleanUnsetAction: "使用默认值",
      cardTitle: "运行工厂",
      defaultValue: (values) =>
        `默认值：${values.length === 1 ? values[0] : values.join("、")}`,
      directoryPathPlaceholder: "输入目录路径。",
      emptyParametersState: "这个工厂无需额外参数即可运行。",
      exampleStdin: (value) => `stdin：${value}`,
      examplesTitle: "示例",
      filePathPlaceholder: "输入文件路径。",
      loadingState: "正在加载当前工厂的调用契约...",
      namedBinding: (name) => `命名参数：--${name}`,
      outputContentType: (value) => `内容类型：${value}`,
      outputFileExtension: (value) => `文件扩展名：${value}`,
      outputHintTitle: "输出提示",
      outputModeLabel: (mode) => `输出模式：${mode}`,
      outputPathParameter: (name) => `输出路径参数：${name}`,
      pathPlaceholder: "输入路径。",
      positionalBinding: (position) => `位置参数 ${position}`,
      primaryResultReady: "主要结果",
      removeRepeatedValue: (label, position) => `移除${label}值 ${position}`,
      requiredAffordance: "必填",
      selectOptionPlaceholder: "选择一个值",
      statusMessages: {
        errorFallback: "无法运行该工厂。请稍后再试。",
        runtimeFailed: (status) => `调用以 ${status} 状态结束。`,
        submitting: "正在调用当前工厂...",
        success: (traceId) => `工厂调用已开始。追踪 ID：${traceId}。`,
        validationFailed: "调用前请先修正高亮参数。",
      },
      stdinBinding: "支持从 stdin 读取输入。",
      submitAction: "运行工厂",
      submittingAction: "正在运行...",
      textPlaceholder: "输入一个值。",
      validationMessages: {
        repeatedItemRequired: "每个重复值都必须为非空。",
        requiredField: (label) => `调用前请输入${label}。`,
      },
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

export function getSubmitWorkMessages(
  locale?: string | null,
): SubmitWorkMessages {
  return resolveLocalizedMessages(submitWorkMessagesByLocale, locale);
}

export { submitWorkMessagesByLocale };
