import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { FactoryGraphBulkSelectionItemKind } from "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";

export interface GraphBulkSelectionDetailMessages {
  docKindLabel: string;
  edgeKindLabel: string;
  heading: string;
  resourceKindLabel: string;
  selectedItemCountLabel: string;
  selectedItemCountValue: (count: number) => string;
  summaryRegionLabel: string;
  unknownKindLabel: string;
  workerKindLabel: string;
  workStateKindLabel: string;
  workstationKindLabel: string;
  workTypeKindLabel: string;
}

const graphBulkSelectionDetailMessagesByLocale = {
  en: {
    docKindLabel: "Docs",
    edgeKindLabel: "Edges",
    heading: "Multiple graph items selected",
    resourceKindLabel: "Resources",
    selectedItemCountLabel: "Selected items",
    selectedItemCountValue: (count: number) => String(count),
    summaryRegionLabel: "Graph bulk selection summary",
    unknownKindLabel: "Other graph items",
    workerKindLabel: "Workers",
    workStateKindLabel: "Work states",
    workstationKindLabel: "Workstations",
    workTypeKindLabel: "Work types",
  },
  "zh-CN": {
    docKindLabel: "文档",
    edgeKindLabel: "边",
    heading: "已选择多个图项",
    resourceKindLabel: "资源",
    selectedItemCountLabel: "已选项目",
    selectedItemCountValue: (count: number) => String(count),
    summaryRegionLabel: "图批量选择摘要",
    unknownKindLabel: "其他图项",
    workerKindLabel: "工作者",
    workStateKindLabel: "工作状态",
    workstationKindLabel: "工作站",
    workTypeKindLabel: "工作类型",
  },
  ko: {
    docKindLabel: "문서",
    edgeKindLabel: "엣지",
    heading: "여러 그래프 항목이 선택됨",
    resourceKindLabel: "리소스",
    selectedItemCountLabel: "선택된 항목",
    selectedItemCountValue: (count: number) => String(count),
    summaryRegionLabel: "그래프 다중 선택 요약",
    unknownKindLabel: "기타 그래프 항목",
    workerKindLabel: "워커",
    workStateKindLabel: "작업 상태",
    workstationKindLabel: "워크스테이션",
    workTypeKindLabel: "작업 유형",
  },
  ja: {
    docKindLabel: "ドキュメント",
    edgeKindLabel: "エッジ",
    heading: "複数のグラフ項目が選択されています",
    resourceKindLabel: "リソース",
    selectedItemCountLabel: "選択項目",
    selectedItemCountValue: (count: number) => String(count),
    summaryRegionLabel: "グラフ一括選択サマリー",
    unknownKindLabel: "その他のグラフ項目",
    workerKindLabel: "ワーカー",
    workStateKindLabel: "作業状態",
    workstationKindLabel: "ワークステーション",
    workTypeKindLabel: "作業タイプ",
  },
} satisfies LocalizedMessages<GraphBulkSelectionDetailMessages>;

export const getGraphBulkSelectionDetailMessages = (locale?: string | null) =>
  resolveLocalizedMessages(graphBulkSelectionDetailMessagesByLocale, locale);

export function graphBulkSelectionKindLabel(
  messages: GraphBulkSelectionDetailMessages,
  kind: FactoryGraphBulkSelectionItemKind,
): string {
  switch (kind) {
    case "doc":
      return messages.docKindLabel;
    case "edge":
      return messages.edgeKindLabel;
    case "resource":
      return messages.resourceKindLabel;
    case "worker":
      return messages.workerKindLabel;
    case "work-state":
      return messages.workStateKindLabel;
    case "workstation":
      return messages.workstationKindLabel;
    case "work-type":
      return messages.workTypeKindLabel;
    default:
      return messages.unknownKindLabel;
  }
}
