# Table primitives and DataTable

Domain-free semantic table primitives and a generic `DataTable` for rendering
arbitrary row objects through host-provided column definitions and render
callbacks. Use these components when you need accessible tabular data display
without importing dashboard routes, API clients, generated OpenAPI types, or app
localization providers.

## Required setup

Import package styles once in your host application before rendering tables:

```css
@import "@you-agent-factory/components/styles.css";
```

Table components depend on package role tokens (`--color-outline`,
`--color-on-surface`, `--color-af-overlay`, typography roles) and Tailwind
utility classes compiled from those tokens. They do not require dashboard
`styles.css`, dashboard providers, React Query, Zustand, or app i18n context.

## Import paths

Prefer the data-display category entrypoint:

```ts
import {
  DataTable,
  type DataTableColumn,
  type DataTableProps,
  type DataTableState,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  type TableSize,
  tableCellTruncateClassName,
  tableCellWrapClassName,
  tableMinWidthWideClassName,
  tableNarrowContainerClassName,
} from "@you-agent-factory/components/data-display";
```

The same exports are also available from the package root:

```ts
import { DataTable } from "@you-agent-factory/components";
```

## Host ownership

The package owns presentation markup, semantic table structure, state-region
semantics, density variants, and scroll containment helpers. Host applications
own everything domain-specific:

| Concern | Owner |
| --- | --- |
| User-visible copy (headers, empty/loading/error messages, cell labels) | Host |
| Row data, domain models, and stable row identity | Host |
| Data fetching, caching, retries, and error mapping | Host |
| Formatting, translations, and business display rules | Host |
| Sorting policy, pagination, selection, and row actions | Host |
| Which `state` value to render (`loading`, `empty`, `error`, `success`) | Host |

Pass data, copy, and callbacks into `DataTable` as props. Do not expect the
package to read from your routers, stores, API layers, or i18n providers.

## Generic row contract

`DataTable` is generic over `Row`. The host defines the row shape and supplies:

1. **`data`** — array of row objects for the success state.
2. **`columns`** — column definitions with stable `id`, visible `header`, and a
   `cell` render callback.
3. **`getRowKey`** — returns a stable string key per row for React list identity.

```tsx
interface ProductRow {
  id: string;
  name: string;
  status: "active" | "draft";
}

const columns: DataTableColumn<ProductRow>[] = [
  {
    id: "name",
    header: "Product",
    cell: (row) => <span className="font-medium">{row.name}</span>,
  },
  {
    id: "status",
    header: "Status",
    cell: (row) => (row.status === "active" ? "Active" : "Draft"),
  },
];

<DataTable<ProductRow>
  ariaLabel="Product catalog"
  columns={columns}
  data={products}
  emptyMessage="No products match the current filters"
  getRowKey={(row) => row.id}
  state="success"
/>
```

### Column definitions

| Field | Purpose |
| --- | --- |
| `id` | Stable column identifier used for React keys. |
| `header` | Visible column heading (`ReactNode`). Host supplies copy and formatting. |
| `cell` | `(row: Row) => ReactNode` callback that renders one cell per row. |
| `headerClassName` | Optional class names for the `<th>` element. |
| `cellClassName` | Optional class names for the `<td>` element (use layout helpers for long cells). |

### Row keys

`getRowKey` must return a stable, unique string for each row in the current
dataset. Use domain identifiers (for example record IDs), not array indexes,
when rows can be reordered, filtered, or updated.

## Explicit data states

`DataTable` does not fetch data or read application context. The host sets
`state` explicitly:

| `state` | When to use | Host copy props |
| --- | --- | --- |
| `loading` | Data is in flight | `loadingMessage` |
| `empty` | Fetch succeeded with zero rows, or filters exclude all rows | `emptyMessage` |
| `error` | Fetch or mapping failed | `errorMessage` |
| `success` | Rows are ready to render | none required; uses `data` and `columns` |

```tsx
<DataTable
  ariaLabel="Product catalog"
  columns={columns}
  data={[]}
  emptyMessage="No products match the current filters"
  errorMessage="Unable to load product catalog data"
  getRowKey={(row) => row.id}
  loadingMessage="Loading product catalog"
  state={isLoading ? "loading" : hasError ? "error" : rows.length === 0 ? "empty" : "success"}
/>
```

Behavior notes:

- Headers remain visible for `loading`, `empty`, and `error` so the layout stays
  table-compatible.
- `state="success"` with an empty `data` array still renders `emptyMessage` for
  backward compatibility.
- `state="empty"` ignores row data even when `data` is non-empty.

## Table primitives

Use low-level primitives when you need custom body composition beyond
`DataTable`:

```tsx
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@you-agent-factory/components/data-display";

<Table aria-label="Custom table" size="dense">
  <TableCaption>Host-provided caption</TableCaption>
  <TableHeader>
    <TableRow>
      <TableHead scope="col">Name</TableHead>
    </TableRow>
  </TableHeader>
  <TableBody>
    <TableRow>
      <TableCell>Host-provided cell</TableCell>
    </TableRow>
  </TableBody>
</Table>
```

`Table` wraps the native `<table>` in a scroll container with
`overflow-x-auto`, rounded border, and optional `size` (`default` | `dense`).

## Density, long cells, and narrow viewports

### Dense mode

Set `size="dense"` on `DataTable` or `Table` for compact header and cell
padding. Dense spacing uses `data-size="dense"` on the scroll container and
`group-data-[size=dense]/table:` utility classes on header and cell elements.

### Long cell content

Export layout helpers from the data-display entrypoint:

| Helper | Purpose |
| --- | --- |
| `tableCellWrapClassName` | Wrap long values with `overflow-wrap: anywhere` inside the table scroller. |
| `tableCellTruncateClassName` | Truncate overflowing values with ellipsis. |
| `tableMinWidthWideClassName` | Minimum table width (`min-w-2xl`) for wide datasets. |
| `tableNarrowContainerClassName` | Scroll container classes (`min-w-0 overscroll-x-contain`) for narrow parents. |

Apply helpers through `column.cellClassName` or `column.headerClassName`:

```tsx
const columns = [
  {
    id: "traceId",
    header: "Trace ID",
    cellClassName: tableCellTruncateClassName,
    cell: (row) => row.traceId,
  },
  {
    id: "notes",
    header: "Notes",
    cellClassName: tableCellWrapClassName,
    cell: (row) => row.notes,
  },
];
```

### Narrow viewports

Pair `tableNarrowContainerClassName` on the table container with
`tableMinWidthWideClassName` on the table element so wide datasets scroll
horizontally inside the table region instead of forcing page-level overflow.

## Key props

### `DataTable`

| Prop | Purpose |
| --- | --- |
| `columns` | Required column definitions with headers and cell callbacks. |
| `data` | Row array for the success state. |
| `getRowKey` | Stable row identity callback. |
| `state` | `loading` \| `empty` \| `error` \| `success` (default `success`). |
| `loadingMessage` / `emptyMessage` / `errorMessage` | Host-provided copy for non-success states. |
| `ariaLabel` | Accessible name when no visible caption names the table. |
| `caption` | Optional `<caption>` content below the table. |
| `size` | `default` or `dense` table spacing. |
| `rowClassName` | Optional per-row class callback. |
| `tableClassName` | Class names on the `<table>` element. |
| `containerClassName` / `containerProps` | Scroll container overrides (for narrow layouts). |

### `Table` primitives

| Prop | Purpose |
| --- | --- |
| `size` | `default` or `dense` spacing variant. |
| `containerClassName` / `containerProps` | Scroll container overrides. |

## Accessibility expectations

- Success state renders semantic `<table>`, `<thead>`, `<tbody>`, `<tr>`, `<th
  scope="col">`, and `<td>` structure.
- Provide `ariaLabel` when the table has no visible caption or external heading
  that names it.
- `loading` renders `role="status"` with `aria-busy="true"` and non-interactive
  placeholder bars (`aria-hidden`) so loading does not masquerade as row data.
- `empty` renders `role="status"` with polite live-region semantics.
- `error` renders `role="alert"` with assertive live-region semantics.
- Interactive controls inside cells (buttons, links) must include accessible
  names from the host (`aria-label` or visible text).
- Dense mode preserves readable focus indicators on interactive controls inside
  cells.

## Storybook visual reference

Package Storybook lives under `Data Display/DataTable`. Stories use package
imports and package token decorators only — no dashboard providers.

| Story | Storybook id | Demonstrates |
| --- | --- | --- |
| Success | `data-display-datatable--success` | Callback-driven generic rows |
| Loading | `data-display-datatable--loading` | Loading status region + placeholders |
| Empty | `data-display-datatable--empty` | Host-provided empty copy |
| Error | `data-display-datatable--error-state` | Host-provided error alert |
| Dense | `data-display-datatable--dense` | Compact row spacing with operable controls |
| Long cell | `data-display-datatable--long-cell` | Wrap and truncate containment |
| Narrow viewport | `data-display-datatable--narrow-viewport` | Horizontal scroll on small widths |

Run package Storybook locally:

```bash
cd ui/packages/components
bun run storybook
```

Browser verification for documented stories:

```bash
cd ui/packages/components
bun run verify:storybook-browser
```

## Allowed dependencies

Table source may import:

- Package utilities (`cn` from `@you-agent-factory/components/utilities`)
- Package token CSS via the host `styles.css` import
- React and `react-dom` peer dependencies

Table source must **not** import:

- Dashboard feature modules, routes, or providers
- Generated OpenAPI clients or dashboard API adapters
- React Query, Zustand, Monaco, Sonner, or dashboard i18n/session providers
- Factory, work, session, provider, or other domain row types

`check:package-boundary` enforces these rules in CI.

## Dashboard integration note

The dashboard keeps data fetching, translations, and business formatting in
feature code. Dashboard tables import primitives and `DataTable` from
`@you-agent-factory/components/data-display` (or through thin dashboard
re-exports) and pass host copy, row data, and render callbacks as props.

When adopting tables outside the dashboard, compose `DataTable` directly and
supply your own messages, row keys, column callbacks, and `state` transitions.
