import type { Meta, StoryObj } from "@storybook/react-vite";

import { DataTable, type DataTableColumn } from "./data-table";
import {
  tableCellTruncateClassName,
  tableCellWrapClassName,
  tableMinWidthWideClassName,
  tableNarrowContainerClassName,
} from "./table-layout";

interface ProductRow {
  category: string;
  id: string;
  name: string;
  status: "active" | "draft";
}

const productRows: ProductRow[] = [
  {
    id: "product-1",
    name: "Signal Router",
    category: "Infrastructure",
    status: "active",
  },
  {
    id: "product-2",
    name: "Review Queue",
    category: "Operations",
    status: "draft",
  },
];

const productColumns: DataTableColumn<ProductRow>[] = [
  {
    id: "name",
    header: "Product",
    cell: (row) => (
      <span className="font-medium text-on-surface">{row.name}</span>
    ),
  },
  {
    id: "category",
    header: "Category",
    cell: (row) => row.category,
  },
  {
    id: "status",
    header: "Status",
    cell: (row) => (
      <span
        className={
          row.status === "active" ? "text-af-success" : "text-af-text-subtle"
        }
      >
        {row.status === "active" ? "Active" : "Draft"}
      </span>
    ),
  },
];

const meta = {
  title: "Data Display/DataTable",
  component: DataTable,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof DataTable>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Success: Story = {
  args: {
    ariaLabel: "Product catalog",
    caption: "Callback-driven rows rendered from host-provided data",
    columns: productColumns as DataTableColumn<unknown>[],
    data: productRows as unknown[],
    getRowKey: (row) => (row as ProductRow).id,
    state: "success",
  },
  render: (args) => (
    <DataTable<ProductRow>
      ariaLabel={args.ariaLabel}
      caption={args.caption}
      columns={productColumns}
      data={productRows}
      getRowKey={(row) => row.id}
      state="success"
    />
  ),
};

export const Loading: Story = {
  args: {
    ariaLabel: "Product catalog loading",
    columns: productColumns as DataTableColumn<unknown>[],
    data: productRows as unknown[],
    getRowKey: (row) => (row as ProductRow).id,
    loadingMessage: "Loading product catalog",
    state: "loading",
  },
  render: (args) => (
    <DataTable<ProductRow>
      ariaLabel={args.ariaLabel}
      columns={productColumns}
      data={productRows}
      getRowKey={(row) => row.id}
      loadingMessage="Loading product catalog"
      state="loading"
    />
  ),
};

export const Empty: Story = {
  args: {
    ariaLabel: "Product catalog empty",
    columns: productColumns as DataTableColumn<unknown>[],
    data: [] as unknown[],
    emptyMessage: "No products match the current filters",
    getRowKey: (row) => (row as ProductRow).id,
    state: "empty",
  },
  render: (args) => (
    <DataTable<ProductRow>
      ariaLabel={args.ariaLabel}
      columns={productColumns}
      data={[]}
      emptyMessage="No products match the current filters"
      getRowKey={(row) => row.id}
      state="empty"
    />
  ),
};

export const ErrorState: Story = {
  name: "Error",
  args: {
    ariaLabel: "Product catalog error",
    columns: productColumns as DataTableColumn<unknown>[],
    data: productRows as unknown[],
    errorMessage: "Unable to load product catalog data",
    getRowKey: (row) => (row as ProductRow).id,
    state: "error",
  },
  render: (args) => (
    <DataTable<ProductRow>
      ariaLabel={args.ariaLabel}
      columns={productColumns}
      data={productRows}
      errorMessage="Unable to load product catalog data"
      getRowKey={(row) => row.id}
      state="error"
    />
  ),
};

interface DenseRow {
  action: string;
  id: string;
  metric: string;
  name: string;
}

const denseRows: DenseRow[] = [
  {
    id: "dense-1",
    name: "Queue depth",
    metric: "128",
    action: "Inspect",
  },
  {
    id: "dense-2",
    name: "Retry budget",
    metric: "12",
    action: "Adjust",
  },
  {
    id: "dense-3",
    name: "Dispatch latency",
    metric: "430ms",
    action: "Trace",
  },
];

const denseColumns: DataTableColumn<DenseRow>[] = [
  {
    id: "name",
    header: "Signal",
    cell: (row) => row.name,
  },
  {
    id: "metric",
    header: "Value",
    cell: (row) => row.metric,
  },
  {
    id: "action",
    header: "Action",
    cell: (row) => (
      <button
        className="rounded-lg border border-outline px-2 py-1 text-xs font-semibold text-on-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring"
        type="button"
      >
        {row.action}
      </button>
    ),
  },
];

export const Dense: Story = {
  args: {
    ariaLabel: "Dense operational table",
    columns: denseColumns as DataTableColumn<unknown>[],
    data: denseRows as unknown[],
    getRowKey: (row) => (row as DenseRow).id,
    size: "dense",
    state: "success",
  },
  render: () => (
    <DataTable<DenseRow>
      ariaLabel="Dense operational table"
      caption="Compact row spacing keeps controls readable in dense dashboards"
      columns={denseColumns}
      data={denseRows}
      getRowKey={(row) => row.id}
      size="dense"
      state="success"
    />
  ),
};

interface LongCellRow {
  id: string;
  notes: string;
  traceId: string;
}

const longCellRows: LongCellRow[] = [
  {
    id: "long-1",
    traceId: "trace-factory-session-7f3c2a91b4d8e6c0",
    notes:
      "Provider session emitted a long diagnostic payload describing retry policy, guard evaluation order, and downstream workstation routing without forcing the surrounding page to scroll horizontally.",
  },
  {
    id: "long-2",
    traceId: "trace-workstation-dispatch-9aa1ff002233445566778899",
    notes:
      "A second row with wrapped copy to show that multi-line cell content stays inside the table scroller.",
  },
];

const longCellColumns: DataTableColumn<LongCellRow>[] = [
  {
    id: "traceId",
    header: "Trace ID",
    cellClassName: tableCellTruncateClassName,
    headerClassName: "max-w-[10rem]",
    cell: (row) => row.traceId,
  },
  {
    id: "notes",
    header: "Notes",
    cellClassName: tableCellWrapClassName,
    cell: (row) => row.notes,
  },
];

export const LongCell: Story = {
  args: {
    ariaLabel: "Long cell content table",
    columns: longCellColumns as DataTableColumn<unknown>[],
    data: longCellRows as unknown[],
    getRowKey: (row) => (row as LongCellRow).id,
    state: "success",
  },
  render: () => (
    <DataTable<LongCellRow>
      ariaLabel="Long cell content table"
      caption="Wrap and truncate helpers keep long values inside the table container"
      columns={longCellColumns}
      data={longCellRows}
      getRowKey={(row) => row.id}
      state="success"
    />
  ),
};

const narrowViewportColumns: DataTableColumn<ProductRow>[] = [
  ...productColumns,
  {
    id: "owner",
    header: "Owner",
    cell: () => "Platform Ops",
  },
  {
    id: "region",
    header: "Region",
    cell: () => "us-west-2",
  },
];

export const NarrowViewport: Story = {
  args: {
    ariaLabel: "Narrow viewport table",
    columns: narrowViewportColumns as DataTableColumn<unknown>[],
    data: productRows as unknown[],
    getRowKey: (row) => (row as ProductRow).id,
    state: "success",
  },
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <div className="max-w-xs">
      <DataTable<ProductRow>
        ariaLabel="Narrow viewport table"
        caption="Horizontal scroll keeps wide tables reachable on narrow viewports"
        columns={narrowViewportColumns}
        containerClassName={tableNarrowContainerClassName}
        data={productRows}
        getRowKey={(row) => row.id}
        state="success"
        tableClassName={tableMinWidthWideClassName}
      />
    </div>
  ),
};
