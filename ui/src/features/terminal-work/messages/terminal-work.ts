import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";
import type { TerminalWorkStatus } from "../lib/types";

export type TerminalWorkMessageStatus = TerminalWorkStatus;

export interface TerminalWorkMessages {
  cardTitle: string;
  disclosureLabel: (expanded: boolean) => string;
  emptyState: (status: TerminalWorkMessageStatus) => string;
  historyProgressLabel: (
    shownCount: number,
    totalCount: number,
    remainingCount: number,
  ) => string;
  iconLabel: (status: TerminalWorkMessageStatus) => string;
  itemCountLabel: (count: number) => string;
  legendLabel: string;
  openWorkItemAction: string;
  rowTitle: (status: TerminalWorkMessageStatus) => string;
  showMoreHistoryAction: (remainingCount: number) => string;
  selectWorkItemLabel: (label: string) => string;
  sessionSummaryFallback: (status: TerminalWorkMessageStatus) => string;
  selectedWorkItemAction: string;
  selectedWorkItemLabel: (workID: string) => string;
  summary: (status: TerminalWorkMessageStatus, workstation: string) => string;
  workIDLabel: (workID: string) => string;
}

const terminalWorkMessagesByLocale: LocalizedMessages<TerminalWorkMessages> = {
  en: {
    cardTitle: "Terminal work outcomes",
    disclosureLabel: (expanded) => (expanded ? "Collapse" : "Expand"),
    emptyState: (status) =>
      `No ${englishTerminalStatusLabel(status).toLowerCase()} work recorded yet.`,
    historyProgressLabel: (shownCount, totalCount, remainingCount) =>
      `Showing ${shownCount} of ${totalCount} items. ${remainingCount} remaining.`,
    iconLabel: (status) => `${englishTerminalStatusLabel(status)} work`,
    itemCountLabel: (count) => `${count} ${count === 1 ? "item" : "items"}`,
    legendLabel: "Terminal work status groups",
    openWorkItemAction: "Open work item",
    rowTitle: englishTerminalStatusLabel,
    showMoreHistoryAction: (remainingCount) =>
      `Show ${remainingCount} more item${remainingCount === 1 ? "" : "s"}`,
    selectWorkItemLabel: (label) => `Select work item ${label}`,
    sessionSummaryFallback: (status) =>
      `${englishTerminalStatusLabel(status)} status recorded by session summary.`,
    selectedWorkItemAction: "Work selected",
    selectedWorkItemLabel: (workID) => `Selected Work ID ${workID}`,
    summary: (status, workstation) =>
      `${englishTerminalStatusLabel(status)} at ${workstation}`,
    workIDLabel: (workID) => `Work ID: ${workID}`,
  },
  ja: {
    cardTitle: "ターミナル作業の結果",
    disclosureLabel: (expanded) => (expanded ? "折りたたむ" : "展開"),
    emptyState: (status) =>
      status === "completed"
        ? "完了した作業はまだ記録されていません。"
        : status === "failed"
          ? "失敗した作業はまだ記録されていません。"
          : `${japaneseTerminalStatusLabel(status)}の作業はまだ記録されていません。`,
    historyProgressLabel: (shownCount, totalCount, remainingCount) =>
      `${totalCount}件中${shownCount}件を表示しています。残り${remainingCount}件。`,
    iconLabel: (status) =>
      status === "completed"
        ? "完了した作業"
        : status === "failed"
          ? "失敗した作業"
          : `${japaneseTerminalStatusLabel(status)}の作業`,
    itemCountLabel: (count) => `${count} 件`,
    legendLabel: "ターミナル作業のステータスグループ",
    openWorkItemAction: "作業を開く",
    rowTitle: japaneseTerminalStatusLabel,
    showMoreHistoryAction: (remainingCount) => `残り${remainingCount}件を表示`,
    selectWorkItemLabel: (label) => `作業項目 ${label} を選択`,
    sessionSummaryFallback: (status) =>
      `セッション概要で${japaneseTerminalStatusLabel(status)}ステータスが記録されました。`,
    selectedWorkItemAction: "作業を選択済み",
    selectedWorkItemLabel: (workID) => `選択中の Work ID ${workID}`,
    summary: (status, workstation) =>
      `${japaneseTerminalStatusLabel(status)}: ${workstation}`,
    workIDLabel: (workID) => `Work ID: ${workID}`,
  },
  ko: {
    cardTitle: "터미널 작업 결과",
    disclosureLabel: (expanded) => (expanded ? "접기" : "펼치기"),
    emptyState: (status) =>
      status === "completed"
        ? "완료된 작업이 아직 기록되지 않았습니다."
        : status === "failed"
          ? "실패한 작업이 아직 기록되지 않았습니다."
          : `${koreanTerminalStatusLabel(status)} 작업이 아직 기록되지 않았습니다.`,
    historyProgressLabel: (shownCount, totalCount, remainingCount) =>
      `${totalCount}개 중 ${shownCount}개를 표시하고 있습니다. ${remainingCount}개 남았습니다.`,
    iconLabel: (status) =>
      status === "completed"
        ? "완료된 작업"
        : status === "failed"
          ? "실패한 작업"
          : `${koreanTerminalStatusLabel(status)} 작업`,
    itemCountLabel: (count) => `${count}개 항목`,
    legendLabel: "터미널 작업 상태 그룹",
    openWorkItemAction: "작업 열기",
    rowTitle: koreanTerminalStatusLabel,
    showMoreHistoryAction: (remainingCount) => `${remainingCount}개 더 표시`,
    selectWorkItemLabel: (label) => `작업 항목 ${label} 선택`,
    sessionSummaryFallback: (status) =>
      `세션 요약에서 ${koreanTerminalStatusLabel(status)} 상태가 기록되었습니다.`,
    selectedWorkItemAction: "작업 선택됨",
    selectedWorkItemLabel: (workID) => `선택된 Work ID ${workID}`,
    summary: (status, workstation) =>
      `${koreanTerminalStatusLabel(status)}: ${workstation}`,
    workIDLabel: (workID) => `Work ID: ${workID}`,
  },
  "zh-CN": {
    cardTitle: "终端工作结果",
    disclosureLabel: (expanded) => (expanded ? "折叠" : "展开"),
    emptyState: (status) =>
      status === "completed"
        ? "尚未记录已完成的工作。"
        : status === "failed"
          ? "尚未记录失败的工作。"
          : `尚未记录${chineseTerminalStatusLabel(status)}工作。`,
    historyProgressLabel: (shownCount, totalCount, remainingCount) =>
      `显示 ${totalCount} 个项目中的 ${shownCount} 个。剩余 ${remainingCount} 个。`,
    iconLabel: (status) =>
      status === "completed"
        ? "已完成的工作"
        : status === "failed"
          ? "失败的工作"
          : `${chineseTerminalStatusLabel(status)}工作`,
    itemCountLabel: (count) => `${count} 个项目`,
    legendLabel: "终端工作状态分组",
    openWorkItemAction: "打开工作项",
    rowTitle: chineseTerminalStatusLabel,
    showMoreHistoryAction: (remainingCount) =>
      `再显示 ${remainingCount} 个项目`,
    selectWorkItemLabel: (label) => `选择工作项 ${label}`,
    sessionSummaryFallback: (status) =>
      `会话摘要已记录${chineseTerminalStatusLabel(status)}状态。`,
    selectedWorkItemAction: "工作项已选中",
    selectedWorkItemLabel: (workID) => `已选中的 Work ID ${workID}`,
    summary: (status, workstation) =>
      `${chineseTerminalStatusLabel(status)}：${workstation}`,
    workIDLabel: (workID) => `Work ID：${workID}`,
  },
};

export function getTerminalWorkMessages(
  locale?: string | null,
): TerminalWorkMessages {
  return resolveLocalizedMessages(terminalWorkMessagesByLocale, locale);
}

export { terminalWorkMessagesByLocale };

function englishTerminalStatusLabel(status: TerminalWorkMessageStatus): string {
  switch (status) {
    case "completed":
      return "Completed";
    case "failed":
      return "Failed";
    case "canceled":
      return "Canceled";
    case "terminated":
      return "Terminated";
    case "unknown":
      return "Unknown";
  }
}

function japaneseTerminalStatusLabel(
  status: TerminalWorkMessageStatus,
): string {
  switch (status) {
    case "completed":
      return "完了";
    case "failed":
      return "失敗";
    case "canceled":
      return "キャンセル済み";
    case "terminated":
      return "終了";
    case "unknown":
      return "不明";
  }
}

function koreanTerminalStatusLabel(status: TerminalWorkMessageStatus): string {
  switch (status) {
    case "completed":
      return "완료";
    case "failed":
      return "실패";
    case "canceled":
      return "취소됨";
    case "terminated":
      return "종료됨";
    case "unknown":
      return "알 수 없음";
  }
}

function chineseTerminalStatusLabel(status: TerminalWorkMessageStatus): string {
  switch (status) {
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "canceled":
      return "已取消";
    case "terminated":
      return "已终止";
    case "unknown":
      return "未知";
  }
}
