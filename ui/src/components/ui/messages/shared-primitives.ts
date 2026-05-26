import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface SharedPrimitiveMessages {
  chartCompletedLabel: string;
  chartFailedLabel: string;
  collapseAction: string;
  collapsibleBody: string;
  collapsibleSectionLabel: string;
  confirmExportAction: string;
  dataTableAriaLabel: string;
  dataTableCaption: string;
  detailPanelLabel: string;
  dialogCancelAction: string;
  dialogCloseLabel: string;
  dialogDescription: string;
  dialogExportNotesLabel: string;
  dialogFactoryNameLabel: string;
  dialogOpenAction: string;
  dialogTitle: string;
  disabledAction: string;
  dispatchAcceptedSample: string;
  dispatchColumnLabel: string;
  dispatchFailedSample: string;
  dispatchReviewOneLabel: string;
  dispatchReviewTwoLabel: string;
  durationColumnLabel: string;
  durationLongSample: string;
  durationShortSample: string;
  emptyTableMessage: string;
  expandAction: string;
  formatListEmptyLabel: string;
  formatTraceUnknownLabel: string;
  loadingLabel: string;
  outlineAction: string;
  primaryAction: string;
  requestNameLabel: string;
  requestNamePlaceholder: string;
  requestTextLabel: string;
  requestTextPlaceholder: string;
  resizablePanelsLabel: string;
  secondaryAction: string;
  sessionLogPathTemplate: string;
  showcaseCalendarLabel: string;
  showcaseChartTitle: string;
  showcaseDescription: string;
  showcaseTitle: string;
  sidebarPanelLabel: string;
  statusColumnLabel: string;
  tableCaption: string;
  workTypeLabel: string;
  workTypeStoryOption: string;
  workTypeTaskOption: string;
}

export const sharedPrimitiveMessagesByLocale = {
  en: {
    chartCompletedLabel: "Completed",
    chartFailedLabel: "Failed",
    collapseAction: "Collapse",
    collapsibleBody:
      "Shared disclosure state is ready for section toggles and drill-down controls.",
    collapsibleSectionLabel: "Collapsible section",
    confirmExportAction: "Confirm export",
    dataTableAriaLabel: "Primitive data table showcase",
    dataTableCaption: "Reusable data table helper for dashboard detail grids.",
    detailPanelLabel: "Detail panel",
    dialogCancelAction: "Cancel",
    dialogCloseLabel: "Close dialog",
    dialogDescription:
      "Shared dialog chrome for export and confirmation flows.",
    dialogExportNotesLabel: "Export notes",
    dialogFactoryNameLabel: "Factory name",
    dialogOpenAction: "Open dialog",
    dialogTitle: "Export factory",
    disabledAction: "Disabled action",
    dispatchAcceptedSample: "ACCEPTED",
    dispatchColumnLabel: "Dispatch",
    dispatchFailedSample: "FAILED",
    dispatchReviewOneLabel: "dispatch-review-1",
    dispatchReviewTwoLabel: "dispatch-review-2",
    durationColumnLabel: "Duration",
    durationLongSample: "1.2s",
    durationShortSample: "420ms",
    emptyTableMessage: "No rows available.",
    expandAction: "Expand",
    formatListEmptyLabel: "None",
    formatTraceUnknownLabel: "Unknown",
    loadingLabel: "Loading",
    outlineAction: "Outline",
    primaryAction: "Primary action",
    requestNameLabel: "Request name",
    requestNamePlaceholder: "Name this request",
    requestTextLabel: "Request text",
    requestTextPlaceholder: "Describe the work to run",
    resizablePanelsLabel: "Resizable panels",
    secondaryAction: "Secondary",
    sessionLogPathTemplate:
      "~/.codex/sessions/{{year}}/{{month}}/{{day}}/rollout-{{sessionID}}.jsonl",
    showcaseCalendarLabel: "Primitive calendar showcase",
    showcaseChartTitle: "Primitive chart showcase",
    showcaseDescription:
      "Shared button, field, dialog, chart, table, skeleton, collapsible, calendar, and resizable building blocks.",
    showcaseTitle: "Shared UI primitives",
    sidebarPanelLabel: "Sidebar panel",
    statusColumnLabel: "Status",
    tableCaption: "Primitive table foundation for trace and detail surfaces.",
    workTypeLabel: "Work type",
    workTypeStoryOption: "story",
    workTypeTaskOption: "task",
  },
  ja: {
    chartCompletedLabel: "完了",
    chartFailedLabel: "失敗",
    collapseAction: "折りたたむ",
    collapsibleBody:
      "共有の開閉状態は、セクション切り替えやドリルダウン操作に使えます。",
    collapsibleSectionLabel: "折りたたみセクション",
    confirmExportAction: "エクスポートを確定",
    dataTableAriaLabel: "プリミティブデータテーブルのショーケース",
    dataTableCaption:
      "ダッシュボード詳細グリッド用の再利用可能なデータテーブルヘルパー。",
    detailPanelLabel: "詳細パネル",
    dialogCancelAction: "キャンセル",
    dialogCloseLabel: "ダイアログを閉じる",
    dialogDescription: "エクスポートと確認フロー用の共有ダイアログクローム。",
    dialogExportNotesLabel: "エクスポートメモ",
    dialogFactoryNameLabel: "ファクトリー名",
    dialogOpenAction: "ダイアログを開く",
    dialogTitle: "ファクトリーをエクスポート",
    disabledAction: "無効な操作",
    dispatchAcceptedSample: "承認済み",
    dispatchColumnLabel: "ディスパッチ",
    dispatchFailedSample: "失敗",
    dispatchReviewOneLabel: "dispatch-review-1",
    dispatchReviewTwoLabel: "dispatch-review-2",
    durationColumnLabel: "所要時間",
    durationLongSample: "1.2秒",
    durationShortSample: "420ミリ秒",
    emptyTableMessage: "行はありません。",
    expandAction: "展開",
    formatListEmptyLabel: "なし",
    formatTraceUnknownLabel: "不明",
    loadingLabel: "読み込み中",
    outlineAction: "アウトライン",
    primaryAction: "主要操作",
    requestNameLabel: "リクエスト名",
    requestNamePlaceholder: "このリクエストに名前を付ける",
    requestTextLabel: "リクエスト本文",
    requestTextPlaceholder: "実行する作業を説明",
    resizablePanelsLabel: "サイズ変更可能なパネル",
    secondaryAction: "副操作",
    sessionLogPathTemplate:
      "~/.codex/sessions/{{year}}/{{month}}/{{day}}/rollout-{{sessionID}}.jsonl",
    showcaseCalendarLabel: "プリミティブカレンダーのショーケース",
    showcaseChartTitle: "プリミティブチャートのショーケース",
    showcaseDescription:
      "共有ボタン、フィールド、ダイアログ、チャート、テーブル、スケルトン、開閉、カレンダー、サイズ変更ブロック。",
    showcaseTitle: "共有 UI プリミティブ",
    sidebarPanelLabel: "サイドバーパネル",
    statusColumnLabel: "ステータス",
    tableCaption: "トレースと詳細画面用のプリミティブテーブル基盤。",
    workTypeLabel: "作業タイプ",
    workTypeStoryOption: "ストーリー",
    workTypeTaskOption: "タスク",
  },
  ko: {
    chartCompletedLabel: "완료",
    chartFailedLabel: "실패",
    collapseAction: "접기",
    collapsibleBody:
      "공유 펼침 상태는 섹션 토글과 드릴다운 컨트롤에 사용할 준비가 되어 있습니다.",
    collapsibleSectionLabel: "접을 수 있는 섹션",
    confirmExportAction: "내보내기 확인",
    dataTableAriaLabel: "프리미티브 데이터 테이블 쇼케이스",
    dataTableCaption: "대시보드 상세 그리드용 재사용 데이터 테이블 헬퍼입니다.",
    detailPanelLabel: "상세 패널",
    dialogCancelAction: "취소",
    dialogCloseLabel: "대화상자 닫기",
    dialogDescription: "내보내기와 확인 흐름을 위한 공유 대화상자 크롬입니다.",
    dialogExportNotesLabel: "내보내기 메모",
    dialogFactoryNameLabel: "팩토리 이름",
    dialogOpenAction: "대화상자 열기",
    dialogTitle: "팩토리 내보내기",
    disabledAction: "비활성 작업",
    dispatchAcceptedSample: "수락됨",
    dispatchColumnLabel: "디스패치",
    dispatchFailedSample: "실패",
    dispatchReviewOneLabel: "dispatch-review-1",
    dispatchReviewTwoLabel: "dispatch-review-2",
    durationColumnLabel: "기간",
    durationLongSample: "1.2초",
    durationShortSample: "420밀리초",
    emptyTableMessage: "사용 가능한 행이 없습니다.",
    expandAction: "펼치기",
    formatListEmptyLabel: "없음",
    formatTraceUnknownLabel: "알 수 없음",
    loadingLabel: "로드 중",
    outlineAction: "윤곽선",
    primaryAction: "기본 작업",
    requestNameLabel: "요청 이름",
    requestNamePlaceholder: "이 요청 이름 지정",
    requestTextLabel: "요청 텍스트",
    requestTextPlaceholder: "실행할 작업 설명",
    resizablePanelsLabel: "크기 조정 패널",
    secondaryAction: "보조 작업",
    sessionLogPathTemplate:
      "~/.codex/sessions/{{year}}/{{month}}/{{day}}/rollout-{{sessionID}}.jsonl",
    showcaseCalendarLabel: "프리미티브 캘린더 쇼케이스",
    showcaseChartTitle: "프리미티브 차트 쇼케이스",
    showcaseDescription:
      "공유 버튼, 필드, 대화상자, 차트, 테이블, 스켈레톤, 접기, 캘린더, 크기 조정 빌딩 블록입니다.",
    showcaseTitle: "공유 UI 프리미티브",
    sidebarPanelLabel: "사이드바 패널",
    statusColumnLabel: "상태",
    tableCaption: "트레이스와 상세 화면을 위한 프리미티브 테이블 기반입니다.",
    workTypeLabel: "작업 유형",
    workTypeStoryOption: "스토리",
    workTypeTaskOption: "태스크",
  },
  "zh-CN": {
    chartCompletedLabel: "已完成",
    chartFailedLabel: "失败",
    collapseAction: "收起",
    collapsibleBody: "共享披露状态已可用于分区切换和钻取控件。",
    collapsibleSectionLabel: "可折叠分区",
    confirmExportAction: "确认导出",
    dataTableAriaLabel: "基础数据表展示",
    dataTableCaption: "用于仪表板详情网格的可复用数据表辅助组件。",
    detailPanelLabel: "详情面板",
    dialogCancelAction: "取消",
    dialogCloseLabel: "关闭对话框",
    dialogDescription: "用于导出和确认流程的共享对话框外壳。",
    dialogExportNotesLabel: "导出备注",
    dialogFactoryNameLabel: "工厂名称",
    dialogOpenAction: "打开对话框",
    dialogTitle: "导出工厂",
    disabledAction: "禁用操作",
    dispatchAcceptedSample: "已接受",
    dispatchColumnLabel: "分派",
    dispatchFailedSample: "失败",
    dispatchReviewOneLabel: "dispatch-review-1",
    dispatchReviewTwoLabel: "dispatch-review-2",
    durationColumnLabel: "耗时",
    durationLongSample: "1.2 秒",
    durationShortSample: "420 毫秒",
    emptyTableMessage: "没有可用行。",
    expandAction: "展开",
    formatListEmptyLabel: "无",
    formatTraceUnknownLabel: "未知",
    loadingLabel: "加载中",
    outlineAction: "轮廓",
    primaryAction: "主要操作",
    requestNameLabel: "请求名称",
    requestNamePlaceholder: "命名此请求",
    requestTextLabel: "请求文本",
    requestTextPlaceholder: "描述要运行的工作",
    resizablePanelsLabel: "可调整大小的面板",
    secondaryAction: "次要操作",
    sessionLogPathTemplate:
      "~/.codex/sessions/{{year}}/{{month}}/{{day}}/rollout-{{sessionID}}.jsonl",
    showcaseCalendarLabel: "基础日历展示",
    showcaseChartTitle: "基础图表展示",
    showcaseDescription:
      "共享按钮、字段、对话框、图表、表格、骨架屏、折叠、日历和可调整大小的构建块。",
    showcaseTitle: "共享 UI 基础组件",
    sidebarPanelLabel: "侧边栏面板",
    statusColumnLabel: "状态",
    tableCaption: "用于跟踪和详情界面的基础表格底座。",
    workTypeLabel: "工作类型",
    workTypeStoryOption: "故事",
    workTypeTaskOption: "任务",
  },
} satisfies LocalizedMessages<SharedPrimitiveMessages>;

export function getSharedPrimitiveMessages(
  locale?: string | null,
): SharedPrimitiveMessages {
  return resolveLocalizedMessages(sharedPrimitiveMessagesByLocale, locale);
}
