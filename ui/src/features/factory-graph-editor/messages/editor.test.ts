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
  ] as const)(
    "resolves %s editor catalog copy",
    (locale, expectedToolbarLabel, expectedAddEntityAction, expectedObserveMode) => {
      const messages = getFactoryGraphEditorMessages(locale);

      expect(messages.toolbarAriaLabel).toBe(expectedToolbarLabel);
      expect(messages.addDialogAddEntityAction).toBe(
        expectedAddEntityAction,
      );
      expect(messages.modeObserve).toBe(expectedObserveMode);
    },
  );

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getFactoryGraphEditorMessages("en");

    expect(getFactoryGraphEditorMessages(undefined).toolbarAddLabel).toBe(
      defaultMessages.toolbarAddLabel,
    );
    expect(getFactoryGraphEditorMessages("fr").draftActionsTitle).toBe(
      defaultMessages.draftActionsTitle,
    );
  });
});
