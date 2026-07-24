import { SUPPORTED_LOCALES } from "../../../../../i18n";
import {
  editableConfigurationControlsMessagesByLocale,
  getEditableConfigurationControlsMessages,
} from "../operational/editable-configuration-controls";

describe("getEditableConfigurationControlsMessages", () => {
  it("supports the expected editable-configuration locales", () => {
    expect(
      Object.keys(editableConfigurationControlsMessagesByLocale).sort(),
    ).toEqual([...SUPPORTED_LOCALES].sort());
  });

  it.each([
    ["en", "Discard local changes"],
    ["ja", "ローカル変更を破棄"],
    ["ko", "로컬 변경 사항 취소"],
    ["zh-CN", "放弃本地更改"],
  ] as const)(
    "resolves %s discard action copy",
    (locale, expectedDiscardAction) => {
      expect(
        getEditableConfigurationControlsMessages(locale)
          .discardLocalChangesAction,
      ).toBe(expectedDiscardAction);
    },
  );

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getEditableConfigurationControlsMessages("en");

    expect(getEditableConfigurationControlsMessages(undefined)).toEqual(
      defaultMessages,
    );
    expect(getEditableConfigurationControlsMessages("fr")).toEqual(
      defaultMessages,
    );
  });
});
