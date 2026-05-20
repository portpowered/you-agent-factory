import {
  factoryGraphEditorMessagesByLocale,
  getFactoryGraphEditorMessages,
} from "./editor";

describe("getFactoryGraphEditorMessages", () => {
  it("supports the required factory-graph-editor locales", () => {
    expect(Object.keys(factoryGraphEditorMessagesByLocale).sort()).toEqual(
      ["en", "zh-CN"].sort(),
    );
  });

  it.each([
    ["en", "Factory graph editor tools", "Add entity", "Observe mode"],
    ["zh-CN", "工厂图编辑器工具", "添加实体", "观察模式"],
  ] as const)("resolves %s editor catalog copy", (locale, expectedToolbarLabel, expectedAddEntityAction, expectedObserveMode) => {
    const messages = getFactoryGraphEditorMessages(locale);

    expect(messages.toolbarAriaLabel).toBe(expectedToolbarLabel);
    expect(messages.addDialogAddEntityAction).toBe(expectedAddEntityAction);
    expect(messages.modeObserve).toBe(expectedObserveMode);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getFactoryGraphEditorMessages("en");

    expect(getFactoryGraphEditorMessages(undefined).toolbarAddLabel).toBe(
      defaultMessages.toolbarAddLabel,
    );
    expect(getFactoryGraphEditorMessages("fr").draftActionsTitle).toBe(
      defaultMessages.draftActionsTitle,
    );
  });

  it.each([
    [
      "workstation",
      "Add workstation",
      "Create a pending workstation in the current graph draft.",
    ],
    [
      "worker",
      "Add worker",
      "Create a pending worker in the current graph draft.",
    ],
    [
      "resource",
      "Add resource",
      "Create a pending resource in the current graph draft.",
    ],
    [
      "work-type",
      "Add work type",
      "Define a new work type and its first ordered state.",
    ],
    [
      "work-state",
      "Add work state",
      "Append a new ordered state to an existing work type.",
    ],
  ] as const)("describes English add-dialog copy for %s drafts", (kind, expectedTitle, expectedDescription) => {
    const messages = getFactoryGraphEditorMessages("en");

    expect(messages.addDialogTitle(kind)).toBe(expectedTitle);
    expect(messages.addDialogDescription(kind)).toBe(expectedDescription);
  });

  it.each([
    ["resource", "Resource"],
    ["worker", "Worker"],
    ["workstation", "Workstation"],
    ["work-type", "Work type"],
    ["work-state", "Work state"],
  ] as const)("describes English graph node kind %s", (kind, expectedLabel) => {
    expect(getFactoryGraphEditorMessages("en").kindLabel(kind)).toBe(
      expectedLabel,
    );
  });

  it.each([
    ["active", "Active"],
    ["errored", "Errored"],
    ["idle", "Idle"],
    ["unavailable", "Unavailable"],
  ] as const)("describes English worker status %s", (status, expectedLabel) => {
    expect(getFactoryGraphEditorMessages("en").workerStatusLabel(status)).toBe(
      expectedLabel,
    );
  });

  it("resolves localized function-backed labels", () => {
    const messages = getFactoryGraphEditorMessages("zh-CN");

    expect(messages.addDialogTitle("work-type")).toBe("添加工作类型");
    expect(messages.addDialogTitle("work-state")).toBe("添加工作状态");
    expect(messages.addDialogTitle("worker")).toBe("添加worker");
    expect(messages.addDialogDescription("workstation")).toBe(
      "在当前图草稿中创建一个待处理工作站。",
    );
    expect(messages.addDialogDescription("work-type")).toBe(
      "定义一个新的工作类型及其首个有序状态。",
    );
    expect(messages.addDialogDescription("work-state")).toBe(
      "向现有工作类型追加一个新的有序状态。",
    );
    expect(messages.addDialogDescription("resource")).toBe(
      "在当前图草稿中创建一个待处理的resource。",
    );
    expect(messages.kindLabel("resource")).toBe("资源");
    expect(messages.kindLabel("worker")).toBe("工作者");
    expect(messages.kindLabel("workstation")).toBe("工作站");
    expect(messages.kindLabel("work-type")).toBe("工作类型");
    expect(messages.kindLabel("work-state")).toBe("工作状态");
    expect(messages.toolbarVisibilityToggleLabel(true, "工作者")).toBe(
      "隐藏工作者泳道",
    );
    expect(messages.toolbarVisibilityToggleLabel(false, "资源")).toBe(
      "显示资源泳道",
    );
    expect(messages.workerStatusLabel("active")).toBe("活跃");
    expect(messages.workerStatusLabel("errored")).toBe("错误");
    expect(messages.workerStatusLabel("idle")).toBe("空闲");
    expect(messages.workerStatusLabel("unavailable")).toBe("不可用");
  });
});
