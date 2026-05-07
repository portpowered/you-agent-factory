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
    ["en", "Current selection", "Open trace"],
    ["zh", "当前选择", "打开追踪"],
    ["ko", "현재 선택", "추적 열기"],
    ["ja", "現在の選択", "トレースを開く"],
  ] as const)("resolves %s catalog copy", (locale, expectedTitle, expectedOpenTraceAction) => {
    const messages = getCurrentSelectionShellMessages(locale);

    expect(messages.title).toBe(expectedTitle);
    expect(messages.openTraceAction).toBe(expectedOpenTraceAction);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getCurrentSelectionShellMessages("en");

    expect(getCurrentSelectionShellMessages(undefined).title).toBe(
      defaultMessages.title,
    );
    expect(getCurrentSelectionShellMessages("fr").title).toBe(
      defaultMessages.title,
    );
    expect(getCurrentSelectionShellMessages("fr").failureReasonLabel).toBe(
      defaultMessages.failureReasonLabel,
    );
  });
});
