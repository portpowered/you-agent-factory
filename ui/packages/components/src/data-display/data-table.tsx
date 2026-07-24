import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../utilities/cn";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  type TableSize,
} from "./table";

export type DataTableState = "empty" | "error" | "loading" | "success";

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
  containerClassName?: string;
  containerProps?: HTMLAttributes<HTMLDivElement>;
  emptyMessage?: ReactNode;
  errorMessage?: ReactNode;
  loadingMessage?: ReactNode;
  rowClassName?: (row: Row) => string | undefined;
  size?: TableSize;
  state?: DataTableState;
  tableClassName?: string;
}

function DataTablePlaceholderBar({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn("animate-pulse rounded-xl bg-af-overlay", className)}
    />
  );
}

function DataTableStatusRow({
  children,
  colSpan,
  role,
  ariaBusy,
  ariaLive,
  cellClassName,
}: {
  children: ReactNode;
  colSpan: number;
  role: "alert" | "status";
  ariaBusy?: boolean;
  ariaLive?: "assertive" | "polite";
  cellClassName?: string;
}) {
  return (
    <TableRow>
      <TableCell className={cellClassName} colSpan={colSpan}>
        <div aria-busy={ariaBusy || undefined} aria-live={ariaLive} role={role}>
          {children}
        </div>
      </TableCell>
    </TableRow>
  );
}

export function DataTable<Row>({
  columns,
  data,
  getRowKey,
  ariaLabel,
  caption,
  containerClassName,
  containerProps,
  emptyMessage,
  errorMessage,
  loadingMessage,
  rowClassName,
  size,
  state = "success",
  tableClassName,
}: DataTableProps<Row>) {
  const renderBody = () => {
    if (state === "loading") {
      return (
        <DataTableStatusRow
          ariaBusy
          ariaLive="polite"
          colSpan={columns.length}
          role="status"
        >
          <div className="grid gap-3 py-1">
            {loadingMessage}
            <div aria-hidden="true" className="grid gap-2">
              <DataTablePlaceholderBar className="h-4 w-full max-w-48" />
              <DataTablePlaceholderBar className="h-8 w-full" />
              <DataTablePlaceholderBar className="h-4 w-full max-w-48" />
            </div>
          </div>
        </DataTableStatusRow>
      );
    }

    if (state === "error") {
      return (
        <DataTableStatusRow
          ariaLive="assertive"
          cellClassName="text-af-text-subtle"
          colSpan={columns.length}
          role="alert"
        >
          {errorMessage}
        </DataTableStatusRow>
      );
    }

    if (state === "empty" || (state === "success" && data.length === 0)) {
      return (
        <DataTableStatusRow
          ariaLive="polite"
          cellClassName="text-af-text-subtle"
          colSpan={columns.length}
          role="status"
        >
          {emptyMessage}
        </DataTableStatusRow>
      );
    }

    return data.map((row) => (
      <TableRow className={rowClassName?.(row)} key={getRowKey(row)}>
        {columns.map((column) => (
          <TableCell className={column.cellClassName} key={column.id}>
            {column.cell(row)}
          </TableCell>
        ))}
      </TableRow>
    ));
  };

  return (
    <Table
      aria-label={ariaLabel}
      className={tableClassName}
      containerClassName={containerClassName}
      containerProps={containerProps}
      size={size}
    >
      {caption ? <TableCaption>{caption}</TableCaption> : null}
      <TableHeader>
        <TableRow>
          {columns.map((column) => (
            <TableHead
              className={column.headerClassName}
              key={column.id}
              scope="col"
            >
              {column.header}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>{renderBody()}</TableBody>
    </Table>
  );
}
