import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import {
  Table as PackageTable,
  TableBody as PackageTableBody,
  TableCell as PackageTableCell,
  TableHead as PackageTableHead,
  TableHeader as PackageTableHeader,
  TableRow as PackageTableRow,
} from "@you-agent-factory/components/data-display";

import { DataTable } from "./data-table";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./table";

describe("dashboard table package migration", () => {
  it("re-exports package table primitives from dashboard UI entrypoints", () => {
    expect(Table).toBe(PackageTable);
    expect(TableHeader).toBe(PackageTableHeader);
    expect(TableBody).toBe(PackageTableBody);
    expect(TableRow).toBe(PackageTableRow);
    expect(TableHead).toBe(PackageTableHead);
    expect(TableCell).toBe(PackageTableCell);
  });

  it("renders representative migrated table and localized empty data table output", () => {
    render(
      <>
        <Table aria-label="dispatch table">
          <TableHeader>
            <TableRow>
              <TableHead scope="col">Dispatch</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell>dispatch-review-1</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <DataTable
          ariaLabel="empty table"
          columns={[{ cell: () => null, header: "Column", id: "column" }]}
          data={[]}
          getRowKey={() => "unused"}
          locale="zh-CN"
        />
      </>,
    );

    expect(screen.getByRole("table", { name: "dispatch table" })).toBeVisible();
    expect(screen.getByText("dispatch-review-1")).toBeVisible();
    expect(screen.getByText("没有可用行。")).toBeVisible();
  });
});
