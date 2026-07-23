// @vitest-environment happy-dom

import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { DataTable } from "./data-table";
import {
  tableCellTruncateClassName,
  tableCellWrapClassName,
} from "./table-layout";

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

describe("DataTable row rendering", () => {
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
    expect(
      screen.getByRole("columnheader", { name: "Name" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Role" }),
    ).toBeInTheDocument();
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
});

describe("DataTable explicit states", () => {
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
    expect(screen.getByRole("status")).toHaveTextContent("No rows available");
  });

  it("renders an explicit loading state with accessible status semantics", () => {
    renderPackageComponent(
      <DataTable
        ariaLabel="Loading data table"
        columns={sampleColumns}
        data={sampleRows}
        getRowKey={(row) => row.id}
        loadingMessage="Loading table rows"
        state="loading"
      />,
    );

    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-busy", "true");
    expect(status).toHaveTextContent("Loading table rows");
    expect(
      screen.queryByRole("cell", { name: "Alpha" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Name" }),
    ).toBeInTheDocument();
  });

  it("renders an explicit empty state without relying on row data", () => {
    renderPackageComponent(
      <DataTable
        ariaLabel="Empty data table"
        columns={sampleColumns}
        data={sampleRows}
        emptyMessage="No matching rows"
        getRowKey={(row) => row.id}
        state="empty"
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("No matching rows");
    expect(
      screen.queryByRole("cell", { name: "Alpha" }),
    ).not.toBeInTheDocument();
  });

  it("renders an explicit error state with alert semantics", () => {
    renderPackageComponent(
      <DataTable
        ariaLabel="Error data table"
        columns={sampleColumns}
        data={sampleRows}
        errorMessage="Unable to load table data"
        getRowKey={(row) => row.id}
        state="error"
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Unable to load table data",
    );
    expect(
      screen.queryByRole("cell", { name: "Alpha" }),
    ).not.toBeInTheDocument();
  });

  it("renders semantic success state rows when data is present", () => {
    renderPackageComponent(
      <DataTable
        ariaLabel="Success data table"
        columns={sampleColumns}
        data={sampleRows}
        getRowKey={(row) => row.id}
        state="success"
      />,
    );

    expect(screen.getByRole("cell", { name: "Alpha" })).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("DataTable layout", () => {
  it("applies dense spacing through the table size prop", () => {
    renderPackageComponent(
      <DataTable
        ariaLabel="Dense data table"
        columns={sampleColumns}
        data={sampleRows}
        getRowKey={(row) => row.id}
        size="dense"
      />,
    );

    const scroller = screen.getByRole("table", {
      name: "Dense data table",
    }).parentElement;
    expect(scroller).toHaveAttribute("data-size", "dense");
    expect(
      screen.getByRole("columnheader", { name: "Name" }).className,
    ).toContain("group-data-[size=dense]/table:px-3");
  });

  it("renders long cell content with host-provided containment classes", () => {
    const longColumns = [
      {
        id: "notes",
        header: "Notes",
        cellClassName: tableCellWrapClassName,
        cell: () =>
          "A very long diagnostic note that should wrap inside the table cell instead of expanding the page width.",
      },
      {
        id: "trace",
        header: "Trace",
        cellClassName: tableCellTruncateClassName,
        cell: () => "trace-factory-session-7f3c2a91b4d8e6c0-overflowing-value",
      },
    ];

    renderPackageComponent(
      <DataTable
        ariaLabel="Long cell table"
        columns={longColumns}
        data={[{ id: "row-1" }]}
        getRowKey={(row) => row.id}
      />,
    );

    const wrapCell = screen.getByRole("cell", {
      name: /very long diagnostic note/i,
    });
    const truncateCell = screen.getByRole("cell", {
      name: /trace-factory-session/i,
    });

    expect(wrapCell.className).toContain("[overflow-wrap:anywhere]");
    expect(truncateCell.className).toContain("truncate");
  });
});
