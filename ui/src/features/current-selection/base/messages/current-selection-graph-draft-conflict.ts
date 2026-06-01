import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";

export interface CurrentSelectionGraphDraftConflictMessages {
  graphDraftConflictWarningDescription: string;
  graphDraftConflictWarningTitle: string;
}

const currentSelectionGraphDraftConflictMessagesByLocale = {
  en: {
    graphDraftConflictWarningDescription:
      "Your current selection save updated the factory topology. Review or discard your unsaved graph draft before saving the graph.",
    graphDraftConflictWarningTitle: "Graph draft may be stale",
  },
  "zh-CN": {
    graphDraftConflictWarningDescription:
      "当前选择保存已更新工厂拓扑。在保存图之前，请查看或放弃未保存的图草稿。",
    graphDraftConflictWarningTitle: "图草稿可能已过期",
  },
  ko: {
    graphDraftConflictWarningDescription:
      "현재 선택 저장으로 공장 토폴로지가 업데이트되었습니다. 그래프를 저장하기 전에 저장되지 않은 그래프 초안을 검토하거나 취소하세요.",
    graphDraftConflictWarningTitle: "그래프 초안이 오래되었을 수 있습니다",
  },
  ja: {
    graphDraftConflictWarningDescription:
      "現在の選択内容の保存により工場トポロジが更新されました。グラフを保存する前に、未保存のグラフ草稿を確認するか破棄してください。",
    graphDraftConflictWarningTitle:
      "グラフ草稿が古くなっている可能性があります",
  },
} satisfies LocalizedMessages<CurrentSelectionGraphDraftConflictMessages>;

export function getCurrentSelectionGraphDraftConflictMessages(
  locale?: string | null,
): CurrentSelectionGraphDraftConflictMessages {
  return resolveLocalizedMessages(
    currentSelectionGraphDraftConflictMessagesByLocale,
    locale,
  );
}

export { currentSelectionGraphDraftConflictMessagesByLocale };
