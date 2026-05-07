import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  getTerminalWorkMessages,
  terminalWorkMessagesByLocale,
} from "./terminal-work";

describe("getTerminalWorkMessages", () => {
  it("supports the expected terminal-work locales", () => {
    expect(Object.keys(terminalWorkMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "Completed and failed work"],
    ["zh", "已完成和失败的工作"],
    ["ko", "완료 및 실패한 작업"],
    ["ja", "完了済みおよび失敗した作業"],
  ] as const)("resolves %s catalog copy", (locale, expectedTitle) => {
    expect(getTerminalWorkMessages(locale).cardTitle).toBe(expectedTitle);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getTerminalWorkMessages("en");

    expect(getTerminalWorkMessages(undefined).cardTitle).toBe(
      defaultMessages.cardTitle,
    );
    expect(getTerminalWorkMessages("fr").cardTitle).toBe(
      defaultMessages.cardTitle,
    );
  });

  it("keeps status-aware labels and fallback copy available through the resolved locale catalog", () => {
    const messages = getTerminalWorkMessages("ja");

    expect(messages.legendLabel).toBe("ターミナル作業の結果");
    expect(messages.rowTitle("completed")).toBe("完了");
    expect(messages.rowTitle("failed")).toBe("失敗");
    expect(messages.iconLabel("completed")).toBe("完了した作業");
    expect(messages.iconLabel("failed")).toBe("失敗した作業");
    expect(messages.disclosureLabel(true)).toBe("折りたたむ");
    expect(messages.disclosureLabel(false)).toBe("展開");
    expect(messages.emptyState("completed")).toBe(
      "完了した作業はまだ記録されていません。",
    );
    expect(messages.sessionSummaryFallback("failed")).toBe(
      "セッション概要で失敗ステータスが記録されました。",
    );
    expect(messages.itemCountLabel(2)).toContain("2");
  });

  it.each([
    ["ko", "completed", "3개 항목"],
    ["zh", "failed", "5 个项目"],
  ] as const)(
    "keeps %s count and empty-state helpers available for coverage-sensitive locales",
    (locale, status, expectedCountLabel) => {
      const messages = getTerminalWorkMessages(locale);

      expect(messages.itemCountLabel(Number.parseInt(expectedCountLabel, 10))).toBe(
        expectedCountLabel,
      );
      expect(messages.emptyState(status)).not.toHaveLength(0);
      expect(messages.sessionSummaryFallback(status)).not.toHaveLength(0);
    },
  );
});
