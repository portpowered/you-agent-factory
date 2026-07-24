import { describe, expect, it } from "vitest";

import { getSubmitWorkMessages } from "./submit-work";

describe("getSubmitWorkMessages shell copy", () => {
  it("formats the submit-work shell copy exposed to customers", () => {
    const messages = getSubmitWorkMessages("en");

    expect(messages.addItemOptionLabel("document")).toBe("Document");
    expect(messages.fileItemDragActive("Image")).toBe(
      "Drop the image file to stage it.",
    );
    expect(messages.fileItemFailure("Audio")).toBe(
      "Retry staging this audio file or choose a different file.",
    );
    expect(messages.fileItemInputLabel("Video")).toBe("Video file");
    expect(messages.fileItemMetadata("brief.md", "text/markdown")).toBe(
      "brief.md (text/markdown)",
    );
    expect(messages.fileItemPlaceholder("Document")).toBe(
      "Drop or choose one document file to stage it for this submission.",
    );
    expect(messages.fileItemReady("brief.md", "text/markdown")).toBe(
      "Staged brief.md (text/markdown).",
    );
    expect(messages.fileItemStaging("brief.md")).toBe("Staging brief.md...");
    expect(messages.removeItemLabel("Text", 2)).toBe("Remove text item 2");
    expect(messages.requestItemLabel(3)).toBe("Text item 3");
    expect(messages.statusMessages.success("trace-9")).toBe(
      "Your request was submitted. Trace ID: trace-9.",
    );
    expect(messages.validationMessages.requestRequired).toBe(
      "Enter a request name before submitting.",
    );
  });
});

describe("getSubmitWorkMessages invocation copy", () => {
  it("formats the invocation affordances exposed by the widget", () => {
    const messages = getSubmitWorkMessages("en");

    expect(messages.invocation.aliases(["body", "prompt"])).toBe(
      "Aliases: body, prompt",
    );
    expect(messages.invocation.defaultValue(["draft"])).toBe("Default: draft");
    expect(messages.invocation.defaultValue(["draft", "final"])).toBe(
      "Default: draft, final",
    );
    expect(messages.invocation.exampleStdin("Summarize this release")).toBe(
      "stdin: Summarize this release",
    );
    expect(messages.invocation.namedBinding("output")).toBe(
      "Named argument: --output",
    );
    expect(messages.invocation.outputContentType("text/markdown")).toBe(
      "Content type: text/markdown",
    );
    expect(messages.invocation.outputFileExtension(".md")).toBe(
      "File extension: .md",
    );
    expect(messages.invocation.outputModeLabel("FILE")).toBe(
      "Output mode: FILE",
    );
    expect(messages.invocation.outputPathParameter("output")).toBe(
      "Output path argument: output",
    );
    expect(messages.invocation.positionalBinding(2)).toBe(
      "Positional argument 2",
    );
    expect(messages.invocation.removeRepeatedValue("tag", 3)).toBe(
      "Remove tag value 3",
    );
    expect(messages.invocation.statusMessages.runtimeFailed("FAILED")).toBe(
      "Invocation finished with status FAILED.",
    );
    expect(messages.invocation.statusMessages.success("trace-7")).toBe(
      "Factory invocation started. Trace ID: trace-7.",
    );
    expect(messages.invocation.validationMessages.requiredField("input")).toBe(
      "Enter input before invoking.",
    );
  });

  it("resolves localized invocation shell copy", () => {
    const messages = getSubmitWorkMessages("zh-CN");

    expect(messages.addItemOptionLabel("image")).toBe("图像");
    expect(messages.fileItemDragActive("图像")).toBe(
      "拖放图像文件以上传暂存。",
    );
    expect(messages.fileItemFailure("音频")).toBe(
      "重新暂存这个音频文件，或改选另一个文件。",
    );
    expect(messages.fileItemInputLabel("视频")).toBe("视频文件");
    expect(messages.fileItemMetadata("简介.md", "text/markdown")).toBe(
      "简介.md（text/markdown）",
    );
    expect(messages.fileItemPlaceholder("文档")).toBe(
      "拖放或选择一个文档文件以暂存到此提交中。",
    );
    expect(messages.fileItemReady("简介.md", "text/markdown")).toBe(
      "已暂存 简介.md（text/markdown）。",
    );
    expect(messages.fileItemStaging("简介.md")).toBe("正在暂存 简介.md...");
    expect(messages.removeItemLabel("文本", 2)).toBe("移除文本项 2");
    expect(messages.invocation.cardTitle).toBe("运行工厂");
    expect(messages.invocation.submitAction).toBe("运行工厂");
    expect(messages.invocation.booleanUnsetAction).toBe("使用默认值");
    expect(messages.invocation.loadingState).toBe(
      "正在加载当前工厂的调用契约...",
    );
    expect(messages.invocation.namedBinding("output")).toBe(
      "命名参数：--output",
    );
    expect(messages.invocation.positionalBinding(4)).toBe("位置参数 4");
    expect(messages.invocation.outputContentType("text/markdown")).toBe(
      "内容类型：text/markdown",
    );
    expect(messages.invocation.outputFileExtension(".md")).toBe(
      "文件扩展名：.md",
    );
    expect(messages.invocation.outputModeLabel("FILE")).toBe("输出模式：FILE");
    expect(messages.invocation.outputPathParameter("output")).toBe(
      "输出路径参数：output",
    );
    expect(messages.invocation.exampleStdin("整理发布摘要")).toBe(
      "stdin：整理发布摘要",
    );
    expect(messages.invocation.removeRepeatedValue("标签", 2)).toBe(
      "移除标签值 2",
    );
    expect(messages.statusMessages.success("trace-zh")).toBe(
      "你的请求已提交。追踪 ID：trace-zh。",
    );
  });
});
