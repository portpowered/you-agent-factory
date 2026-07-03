/** Stable category path for `@you-agent-factory/components/data-display`. */
export const COMPONENTS_CATEGORY = "data-display" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

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
export type { TableProps } from "./table";
