/** Stable category path for `@you-agent-factory/components/data-display`. */
export const COMPONENTS_CATEGORY = "data-display" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export { CodePanel, codePanelVariants } from "./code-panel";
export type { CodePanelProps } from "./code-panel";
export { DescriptionList } from "./description-list";
export type { DescriptionListProps } from "./description-list";

export { DataTable } from "./data-table";
export type {
  DataTableColumn,
  DataTableProps,
  DataTableState,
} from "./data-table";

export {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./table";
export type { TableProps, TableSize } from "./table";

export {
  tableCellTruncateClassName,
  tableCellWrapClassName,
  tableMinWidthWideClassName,
  tableNarrowContainerClassName,
} from "./table-layout";
