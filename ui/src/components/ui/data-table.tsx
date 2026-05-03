import type { ReactNode } from "react";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./table";

export interface DataTableColumn<Row> {
  cell: (row: Row) => ReactNode;
  header: ReactNode;
  id: string;
  cellClassName?: string;
  headerClassName?: string;
}

export interface DataTableProps<Row> {
  columns: DataTableColumn<Row>[];
  data: Row[];
  getRowKey: (row: Row) => string;
  ariaLabel?: string;
  caption?: ReactNode;
  emptyMessage?: ReactNode;
  rowClassName?: (row: Row) => string | undefined;
  tableClassName?: string;
}

export function DataTable<Row>({
  columns,
  data,
  getRowKey,
  ariaLabel,
  caption,
  emptyMessage = "No rows available.",
  rowClassName,
  tableClassName,
}: DataTableProps<Row>) {
  return (
    <Table aria-label={ariaLabel} className={tableClassName}>
      {caption ? <TableCaption>{caption}</TableCaption> : null}
      <TableHeader>
        <TableRow>
          {columns.map((column) => (
            <TableHead className={column.headerClassName} key={column.id} scope="col">
              {column.header}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {data.length > 0 ? (
          data.map((row) => (
            <TableRow className={rowClassName?.(row)} key={getRowKey(row)}>
              {columns.map((column) => (
                <TableCell className={column.cellClassName} key={column.id}>
                  {column.cell(row)}
                </TableCell>
              ))}
            </TableRow>
          ))
        ) : (
          <TableRow>
            <TableCell className="text-af-ink/58" colSpan={columns.length}>
              {emptyMessage}
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  );
}

