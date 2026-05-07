import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CurrentSelectionShellMessages {
  emptyStateGuidance: string;
  redoAction: string;
  redoActionLabel: string;
  title: string;
  undoAction: string;
  undoActionLabel: string;
}

const currentSelectionShellMessagesByLocale = {
  en: {
    emptyStateGuidance:
      "Select a workstation, work item, or state node to inspect live details.",
    redoAction: "Redo",
    redoActionLabel: "Redo selection",
    title: "Current selection",
    undoAction: "Undo",
    undoActionLabel: "Undo selection",
  },
  ja: {
    emptyStateGuidance:
      "ライブの詳細を確認するには、ワークステーション、作業項目、または状態ノードを選択してください。",
    redoAction: "やり直す",
    redoActionLabel: "選択をやり直す",
    title: "現在の選択",
    undoAction: "元に戻す",
    undoActionLabel: "選択を元に戻す",
  },
  ko: {
    emptyStateGuidance:
      "실시간 세부 정보를 확인하려면 워크스테이션, 작업 항목 또는 상태 노드를 선택하세요.",
    redoAction: "다시 실행",
    redoActionLabel: "선택 다시 실행",
    title: "현재 선택",
    undoAction: "실행 취소",
    undoActionLabel: "선택 실행 취소",
  },
  zh: {
    emptyStateGuidance: "选择工作站、工作项或状态节点以查看实时详细信息。",
    redoAction: "重做",
    redoActionLabel: "重做选择",
    title: "当前选择",
    undoAction: "撤销",
    undoActionLabel: "撤销选择",
  },
} satisfies LocalizedMessages<CurrentSelectionShellMessages>;

export function getCurrentSelectionShellMessages(
  locale?: string | null,
): CurrentSelectionShellMessages {
  return resolveLocalizedMessages(currentSelectionShellMessagesByLocale, locale);
}

export { currentSelectionShellMessagesByLocale };
