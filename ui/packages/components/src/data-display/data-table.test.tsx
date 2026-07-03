// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { DataTable } from "./data-table";

interface SampleRow {
  id: string;
  name: string;
}

const sampleRows: SampleRow[] = [
  { id: "row-1", name: "Alpha" },
  { id: "row-2", name: "Beta" },
];

const sampleColumns = [
  {
    id: "name",
    header: "Name",
    cell: (row: SampleRow) => row.name,
  },
];

describe("DataTable", () => {
  it("exports a generic table that renders host-provided rows and empty copy", () => {
    const { rerender } = renderPackageComponent(
      <DataTable
        ariaLabel="Sample data table"
        columns={sampleColumns}
        data={sampleRows}
        getRowKey={(row) => row.id}
      />,
    );

    expect(
      screen.getByRole("table", { name: "Sample data table" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Alpha" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Beta" })).toBeInTheDocument();

    rerender(
      <DataTable
        ariaLabel="Sample data table"
        columns={sampleColumns}
        data={[]}
        emptyMessage="No rows available"
        getRowKey={(row) => row.id}
      />,
    );

    expect(screen.getByText("No rows available")).toBeInTheDocument();
  });
});
