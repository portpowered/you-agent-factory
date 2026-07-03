// @vitest-environment happy-dom

import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { DataTable } from "./data-table";

interface SampleRow {
  id: string;
  name: string;
  role: string;
}

const sampleRows: SampleRow[] = [
  { id: "row-1", name: "Alpha", role: "Owner" },
  { id: "row-2", name: "Beta", role: "Editor" },
];

const sampleColumns = [
  {
    id: "name",
    header: "Name",
    cell: (row: SampleRow) => row.name,
  },
  {
    id: "role",
    header: "Role",
    cell: (row: SampleRow) => (
      <span data-testid={`role-${row.id}`}>{row.role}</span>
    ),
  },
];

describe("DataTable", () => {
  it("renders generic rows through host-provided column definitions", () => {
    renderPackageComponent(
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
    expect(screen.getByRole("columnheader", { name: "Name" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Role" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Alpha" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Beta" })).toBeInTheDocument();
  });

  it("renders callback-driven cell content supplied by the host", () => {
    renderPackageComponent(
      <DataTable
        ariaLabel="Callback-driven table"
        columns={sampleColumns}
        data={sampleRows}
        getRowKey={(row) => row.id}
      />,
    );

    expect(screen.getByTestId("role-row-1")).toHaveTextContent("Owner");
    expect(screen.getByTestId("role-row-2")).toHaveTextContent("Editor");
  });

  it("uses host-provided row keys for stable row identity", () => {
    const getRowKey = vi.fn((row: SampleRow) => row.id);

    const { rerender } = renderPackageComponent(
      <DataTable
        ariaLabel="Stable row keys"
        columns={sampleColumns}
        data={sampleRows}
        getRowKey={getRowKey}
      />,
    );

    expect(getRowKey).toHaveBeenCalledTimes(sampleRows.length);
    expect(getRowKey).toHaveBeenCalledWith(sampleRows[0]);
    expect(getRowKey).toHaveBeenCalledWith(sampleRows[1]);

    const reorderedRows = [sampleRows[1], sampleRows[0]];
    getRowKey.mockClear();

    rerender(
      <DataTable
        ariaLabel="Stable row keys"
        columns={sampleColumns}
        data={reorderedRows}
        getRowKey={getRowKey}
      />,
    );

    expect(getRowKey).toHaveBeenCalledTimes(reorderedRows.length);
    expect(screen.getAllByRole("row")).toHaveLength(3);
    expect(screen.getByTestId("role-row-2")).toHaveTextContent("Editor");
    expect(screen.getByTestId("role-row-1")).toHaveTextContent("Owner");
  });

  it("renders host-provided empty copy when no rows are present", () => {
    renderPackageComponent(
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
