import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export type TerminalWorkMessageStatus = "completed" | "failed";

export interface TerminalWorkMessages {
  cardTitle: string;
  disclosureLabel: (expanded: boolean) => string;
  emptyState: (status: TerminalWorkMessageStatus) => string;
  iconLabel: (status: TerminalWorkMessageStatus) => string;
  itemCountLabel: (count: number) => string;
  legendLabel: string;
  openWorkItemAction: string;
  rowTitle: (status: TerminalWorkMessageStatus) => string;
  selectWorkItemLabel: (label: string) => string;
  sessionSummaryFallback: (status: TerminalWorkMessageStatus) => string;
  selectedWorkItemAction: string;
  selectedWorkItemLabel: (label: string) => string;
  summary: (status: TerminalWorkMessageStatus, workstation: string) => string;
}

const terminalWorkMessagesByLocale: LocalizedMessages<TerminalWorkMessages> = {
  en: {
    cardTitle: "Completed and failed work",
    disclosureLabel: (expanded) => (expanded ? "Collapse" : "Expand"),
    emptyState: (status) =>
      status === "failed"
        ? "No failed work recorded yet."
        : "No completed work recorded yet.",
    iconLabel: (status) =>
      status === "failed" ? "Failed work" : "Completed work",
    itemCountLabel: (count) => `${count} ${count === 1 ? "item" : "items"}`,
    legendLabel: "Terminal work outcomes",
    openWorkItemAction: "Open work item",
    rowTitle: (status) => (status === "failed" ? "Failed" : "Completed"),
    selectWorkItemLabel: (label) => `Select work item ${label}`,
    sessionSummaryFallback: (status) =>
      status === "failed"
        ? "Failed status recorded by session summary."
        : "Completed by session summary.",
    selectedWorkItemAction: "Work selected",
    selectedWorkItemLabel: (label) => `Selected work item ${label}`,
    summary: (status, workstation) =>
      `${status === "failed" ? "Failed" : "Completed"} at ${workstation}`,
  },
  ja: {
    cardTitle: "完了済みおよび失敗した作業",
    disclosureLabel: (expanded) => (expanded ? "折りたたむ" : "展開"),
    emptyState: (status) =>
      status === "failed"
        ? "失敗した作業はまだ記録されていません。"
        : "完了した作業はまだ記録されていません。",
    iconLabel: (status) =>
      status === "failed" ? "失敗した作業" : "完了した作業",
    itemCountLabel: (count) => `${count} 件`,
    legendLabel: "ターミナル作業の結果",
    openWorkItemAction: "作業を開く",
    rowTitle: (status) => (status === "failed" ? "失敗" : "完了"),
    selectWorkItemLabel: (label) => `作業項目 ${label} を選択`,
    sessionSummaryFallback: (status) =>
      status === "failed"
        ? "セッション概要で失敗ステータスが記録されました。"
        : "セッション概要で完了として記録されました。",
    selectedWorkItemAction: "作業を選択済み",
    selectedWorkItemLabel: (label) => `選択中の作業項目 ${label}`,
    summary: (status, workstation) =>
      `${status === "failed" ? "失敗" : "完了"}: ${workstation}`,
  },
  ko: {
    cardTitle: "완료 및 실패한 작업",
    disclosureLabel: (expanded) => (expanded ? "접기" : "펼치기"),
    emptyState: (status) =>
      status === "failed"
        ? "실패한 작업이 아직 기록되지 않았습니다."
        : "완료된 작업이 아직 기록되지 않았습니다.",
    iconLabel: (status) =>
      status === "failed" ? "실패한 작업" : "완료된 작업",
    itemCountLabel: (count) => `${count}개 항목`,
    legendLabel: "터미널 작업 결과",
    openWorkItemAction: "작업 열기",
    rowTitle: (status) => (status === "failed" ? "실패" : "완료"),
    selectWorkItemLabel: (label) => `작업 항목 ${label} 선택`,
    sessionSummaryFallback: (status) =>
      status === "failed"
        ? "세션 요약에서 실패 상태가 기록되었습니다."
        : "세션 요약에서 완료 상태로 기록되었습니다.",
    selectedWorkItemAction: "작업 선택됨",
    selectedWorkItemLabel: (label) => `선택된 작업 항목 ${label}`,
    summary: (status, workstation) =>
      `${status === "failed" ? "실패" : "완료"}: ${workstation}`,
  },
  "zh-CN": {
    cardTitle: "已完成和失败的工作",
    disclosureLabel: (expanded) => (expanded ? "折叠" : "展开"),
    emptyState: (status) =>
      status === "failed" ? "尚未记录失败的工作。" : "尚未记录已完成的工作。",
    iconLabel: (status) =>
      status === "failed" ? "失败的工作" : "已完成的工作",
    itemCountLabel: (count) => `${count} 个项目`,
    legendLabel: "终端工作结果",
    openWorkItemAction: "打开工作项",
    rowTitle: (status) => (status === "failed" ? "失败" : "已完成"),
    selectWorkItemLabel: (label) => `选择工作项 ${label}`,
    sessionSummaryFallback: (status) =>
      status === "failed"
        ? "会话摘要已记录失败状态。"
        : "会话摘要已记录完成状态。",
    selectedWorkItemAction: "工作项已选中",
    selectedWorkItemLabel: (label) => `已选中的工作项 ${label}`,
    summary: (status, workstation) =>
      `${status === "failed" ? "失败" : "已完成"}：${workstation}`,
  },
};

export function getTerminalWorkMessages(
  locale?: string | null,
): TerminalWorkMessages {
  return resolveLocalizedMessages(terminalWorkMessagesByLocale, locale);
}

export { terminalWorkMessagesByLocale };
