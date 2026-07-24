import { SUPPORTED_LOCALES } from "../../../../../i18n";
import {
  currentSelectionGraphDraftConflictMessagesByLocale,
  getCurrentSelectionGraphDraftConflictMessages,
} from "./current-selection-graph-draft-conflict";

describe("getCurrentSelectionGraphDraftConflictMessages", () => {
  it("supports the expected graph-draft conflict locales", () => {
    expect(
      Object.keys(currentSelectionGraphDraftConflictMessagesByLocale).sort(),
    ).toEqual([...SUPPORTED_LOCALES].sort());
  });

  it.each([
    [
      "en",
      "Graph draft may be stale",
      "Your current selection save updated the factory topology. Review or discard your unsaved graph draft before saving the graph.",
    ],
    [
      "ja",
      "グラフ草稿が古くなっている可能性があります",
      "現在の選択内容の保存により工場トポロジが更新されました。グラフを保存する前に、未保存のグラフ草稿を確認するか破棄してください。",
    ],
    [
      "ko",
      "그래프 초안이 오래되었을 수 있습니다",
      "현재 선택 저장으로 공장 토폴로지가 업데이트되었습니다. 그래프를 저장하기 전에 저장되지 않은 그래프 초안을 검토하거나 취소하세요.",
    ],
    [
      "zh-CN",
      "图草稿可能已过期",
      "当前选择保存已更新工厂拓扑。在保存图之前，请查看或放弃未保存的图草稿。",
    ],
  ] as const)(
    "resolves %s graph-draft conflict warning copy",
    (locale, expectedTitle, expectedDescription) => {
      const messages = getCurrentSelectionGraphDraftConflictMessages(locale);

      expect(messages.graphDraftConflictWarningTitle).toBe(expectedTitle);
      expect(messages.graphDraftConflictWarningDescription).toBe(
        expectedDescription,
      );
    },
  );

  it("does not claim the graph draft was discarded or merged", () => {
    for (const locale of SUPPORTED_LOCALES) {
      const { graphDraftConflictWarningDescription } =
        getCurrentSelectionGraphDraftConflictMessages(locale);

      expect(graphDraftConflictWarningDescription).not.toMatch(
        /discarded|merged|已丢弃|已合并|破棄され|병합/i,
      );
    }
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getCurrentSelectionGraphDraftConflictMessages("en");

    expect(getCurrentSelectionGraphDraftConflictMessages(undefined)).toEqual(
      defaultMessages,
    );
    expect(getCurrentSelectionGraphDraftConflictMessages("fr")).toEqual(
      defaultMessages,
    );
  });
});
