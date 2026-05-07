import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  currentSelectionShellMessagesByLocale,
  getCurrentSelectionShellMessages,
} from "./current-selection-shell";

describe("getCurrentSelectionShellMessages", () => {
  it("supports the expected current-selection locales", () => {
    expect(Object.keys(currentSelectionShellMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "Current selection"],
    ["zh", "当前选择"],
    ["ko", "현재 선택"],
    ["ja", "現在の選択"],
  ] as const)("resolves %s catalog copy", (locale, expectedTitle) => {
    expect(getCurrentSelectionShellMessages(locale).title).toBe(expectedTitle);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getCurrentSelectionShellMessages("en");

    expect(getCurrentSelectionShellMessages(undefined).title).toBe(
      defaultMessages.title,
    );
    expect(getCurrentSelectionShellMessages("fr").title).toBe(
      defaultMessages.title,
    );
  });
});
