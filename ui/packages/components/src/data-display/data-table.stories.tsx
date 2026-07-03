import type { Meta, StoryObj } from "@storybook/react-vite";

import { DataTable, type DataTableColumn } from "./data-table";

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
          row.status === "active"
            ? "text-af-success"
            : "text-af-text-subtle"
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

export const Error: Story = {
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
