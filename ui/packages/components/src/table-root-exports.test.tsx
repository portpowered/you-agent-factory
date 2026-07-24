// @vitest-environment happy-dom

import {
  DataTable,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  tableCellWrapClassName,
} from "@you-agent-factory/components";
import * as dataDisplay from "@you-agent-factory/components/data-display";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "./testing/render";

const sampleColumns = [
  { id: "name", header: "Name", cell: (row: { name: string }) => row.name },
];

describe("@you-agent-factory/components table root exports", () => {
  it("imports table primitives, DataTable, and layout helpers from the package root", () => {
    expect(Table).toBeTruthy();
    expect(TableHeader).toBeTruthy();
    expect(TableBody).toBeTruthy();
    expect(TableRow).toBeTruthy();
    expect(TableHead).toBeTruthy();
    expect(TableCell).toBeTruthy();
    expect(typeof DataTable).toBe("function");
    expect(tableCellWrapClassName).toContain("overflow-wrap");
  });

  it("imports the same table surface from the data-display entrypoint", () => {
    expect(dataDisplay.Table).toBe(Table);
    expect(dataDisplay.DataTable).toBe(DataTable);
    expect(dataDisplay.tableCellWrapClassName).toBe(tableCellWrapClassName);
  });

  it("renders exported DataTable from the package root without dashboard providers", () => {
    renderPackageComponent(
      <DataTable
        ariaLabel="Root export table"
        columns={sampleColumns}
        data={[{ id: "row-1", name: "Package row" }]}
        getRowKey={(row) => row.id}
        state="success"
      />,
    );

    expect(
      screen.getByRole("table", { name: "Root export table" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("cell", { name: "Package row" }),
    ).toBeInTheDocument();
  });
});
