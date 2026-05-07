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
    ["en", "Current selection", "Open trace", "Undo", "Redo"],
    ["zh", "当前选择", "打开追踪", "撤销", "重做"],
    ["ko", "현재 선택", "추적 열기", "실행 취소", "다시 실행"],
    ["ja", "現在の選択", "トレースを開く", "元に戻す", "やり直す"],
  ] as const)(
    "resolves %s catalog copy",
    (
      locale,
      expectedTitle,
      expectedOpenTraceAction,
      expectedUndoLabel,
      expectedRedoLabel,
    ) => {
      const messages = getCurrentSelectionShellMessages(locale);

      expect(messages.title).toBe(expectedTitle);
      expect(messages.openTraceAction).toBe(expectedOpenTraceAction);
      expect(messages.undoLabel).toBe(expectedUndoLabel);
      expect(messages.redoLabel).toBe(expectedRedoLabel);
    },
  );

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getCurrentSelectionShellMessages("en");

    expect(getCurrentSelectionShellMessages(undefined)).toEqual(
      defaultMessages,
    );
    expect(getCurrentSelectionShellMessages("fr")).toEqual(defaultMessages);
    expect(getCurrentSelectionShellMessages("fr").failureReasonLabel).toBe(
      defaultMessages.failureReasonLabel,
    );
  });

  it.each([
    ["en", "Undo selection", "Redo selection"],
    ["ja", "選択を元に戻す", "選択をやり直す"],
  ] as const)(
    "keeps %s accessible labels available through the resolved locale catalog",
    (locale, expectedUndoAccessibleLabel, expectedRedoAccessibleLabel) => {
      const messages = getCurrentSelectionShellMessages(locale);

      expect(messages.undoAccessibleLabel).toBe(expectedUndoAccessibleLabel);
      expect(messages.redoAccessibleLabel).toBe(expectedRedoAccessibleLabel);
    },
  );
});
