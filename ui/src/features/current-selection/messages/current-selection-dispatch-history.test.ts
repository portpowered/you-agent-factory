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
    [
      "en",
      "Current dispatch",
      "Request details",
      "Unknown dispatch",
      "Trace IDs",
      "Select work item Active Story",
      "Expand",
    ],
    [
      "zh-CN",
      "当前分派",
      "请求详情",
      "未知分派",
      "追踪 ID",
      "选择工作项 Active Story",
      "展开",
    ],
    [
      "ko",
      "현재 디스패치",
      "요청 세부 정보",
      "알 수 없는 디스패치",
      "추적 ID",
      "작업 항목 Active Story 선택",
      "펼치기",
    ],
    [
      "ja",
      "現在のディスパッチ",
      "リクエストの詳細",
      "不明なディスパッチ",
      "トレース ID",
      "作業項目 Active Story を選択",
      "展開",
    ],
  ] as const)("resolves %s catalog copy", (locale, expectedCurrentDispatchBadge, expectedRequestDetailsTitle, expectedUnknownDispatchTitle, expectedTraceIdsLabel, expectedSelectWorkItemLabel, expectedExpandAction) => {
    const messages = getCurrentSelectionDispatchHistoryMessages(locale);

    expect(messages.currentDispatchBadge).toBe(expectedCurrentDispatchBadge);
    expect(messages.requestDetailsTitle).toBe(expectedRequestDetailsTitle);
    expect(messages.unknownDispatchTitle).toBe(expectedUnknownDispatchTitle);
    expect(messages.traceIdsLabel).toBe(expectedTraceIdsLabel);
    expect(messages.selectWorkItemAccessibleLabel("Active Story")).toBe(
      expectedSelectWorkItemLabel,
    );
    expect(messages.expandAction).toBe(expectedExpandAction);
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
    expect(
      getCurrentSelectionDispatchHistoryMessages("fr").noScriptAttemptRecordedYet,
    ).toBe(defaultMessages.noScriptAttemptRecordedYet);
  });
});
