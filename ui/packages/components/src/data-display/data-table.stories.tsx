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
  },
  render: (args) => (
    <DataTable<ProductRow>
      ariaLabel={args.ariaLabel}
      caption={args.caption}
      columns={productColumns}
      data={productRows}
      getRowKey={(row) => row.id}
    />
  ),
};
