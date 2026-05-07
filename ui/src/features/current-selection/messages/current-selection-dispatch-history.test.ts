import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  currentSelectionDispatchHistoryMessagesByLocale,
  getCurrentSelectionDispatchHistoryMessages,
} from "./current-selection-dispatch-history";

describe("getCurrentSelectionDispatchHistoryMessages", () => {
  it("supports the expected current-selection locales", () => {
    expect(
      Object.keys(currentSelectionDispatchHistoryMessagesByLocale).sort(),
    ).toEqual([...SUPPORTED_LOCALES].sort());
  });

  it.each([
    ["en", "Current dispatch", "Request details", "Unknown dispatch"],
    ["zh", "当前分派", "请求详情", "未知分派"],
    ["ko", "현재 디스패치", "요청 세부 정보", "알 수 없는 디스패치"],
    ["ja", "現在のディスパッチ", "リクエストの詳細", "不明なディスパッチ"],
  ] as const)("resolves %s catalog copy", (locale, expectedCurrentDispatchBadge, expectedRequestDetailsTitle, expectedUnknownDispatchTitle) => {
    const messages = getCurrentSelectionDispatchHistoryMessages(locale);

    expect(messages.currentDispatchBadge).toBe(expectedCurrentDispatchBadge);
    expect(messages.requestDetailsTitle).toBe(expectedRequestDetailsTitle);
    expect(messages.unknownDispatchTitle).toBe(expectedUnknownDispatchTitle);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getCurrentSelectionDispatchHistoryMessages("en");

    expect(getCurrentSelectionDispatchHistoryMessages(undefined)).toEqual(
      defaultMessages,
    );
    expect(getCurrentSelectionDispatchHistoryMessages("fr")).toEqual(
      defaultMessages,
    );
    expect(
      getCurrentSelectionDispatchHistoryMessages("fr").noScriptResponseYet,
    ).toBe(defaultMessages.noScriptResponseYet);
  });
});
