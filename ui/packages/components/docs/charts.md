# Charts

Domain-neutral chart presentation primitives built on [Recharts](https://recharts.org/).
Import chart components from `@you-agent-factory/components/charts`. The package
owns markup, tokens, accessible chart chrome, tooltip and legend content, and
generic state panels. Host applications own data fetching, domain series
generation, localized copy, and semantic color mapping.

## Install and imports

Recharts is a regular dependency of `@you-agent-factory/components`. Host apps
that render charts should also depend on `recharts` so TypeScript can resolve
Recharts child components (`LineChart`, `Line`, `XAxis`, and so on).

```ts
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartStatePanel,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
  type ChartPresentation,
  type ChartStatePanelProps,
  type ChartStateStatus,
} from "@you-agent-factory/components/charts";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
```

Import package styles once in your host CSS before rendering charts:

```css
@import "@you-agent-factory/components/styles.css";
```

## Minimal Recharts composition

`ChartContainer` wraps a Recharts chart, exposes an accessible chart region,
and injects CSS color variables from your config. Series strokes and fills
reference those variables.

```tsx
const chartConfig: ChartConfig = {
  alpha: { color: "#3366cc", label: "Alpha series" },
  beta: { color: "#cc6633", label: "Beta series" },
};

const chartData = [
  { alpha: 4, beta: 2, tick: 1 },
  { alpha: 6, beta: 3, tick: 2 },
  { alpha: 5, beta: 4, tick: 3 },
];

function ThroughputChart() {
  return (
    <ChartContainer config={chartConfig} title="Sample throughput chart">
      <LineChart data={chartData}>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="tick" />
        <YAxis />
        <ChartTooltip content={<ChartTooltipContent />} />
        <Line dataKey="alpha" stroke="var(--color-alpha)" type="monotone" />
        <Line dataKey="beta" stroke="var(--color-beta)" type="monotone" />
      </LineChart>
    </ChartContainer>
  );
}
```

The container renders `role="img"` with `aria-label` set from the required
`title` prop. Recharts children read `--color-<seriesKey>` variables that
`ChartContainer` derives from `config` entries.

## Chart config

`ChartConfig` is a record of series keys to `ChartConfigEntry` objects:

| Field   | Purpose |
| ------- | ------- |
| `color` | Hex or CSS color used for the `--color-<key>` variable and legend swatches |
| `label` | Human-readable series name shown in tooltips and legends |

```ts
const chartConfig: ChartConfig = {
  queued: { color: "#5c6bc0", label: "Queued work" },
  completed: { color: "#43a047", label: "Completed work" },
};
```

Map domain data into `ChartConfig` in the host app. The chart package does not
import work, session, provider, outcome, dashboard, or generated API types.
Dashboard work-outcome charts, for example, keep semantic role definitions and
series generation in dashboard feature code and pass a prepared config into
`ChartContainer`.

## Presentation: standalone versus embedded

`ChartPresentation` is `"standalone"` (default) or `"embedded"`.

- **Standalone** — Fixed-height bordered card (`h-[18rem]`) suited to docs,
  Storybook, and full-width dashboard panels.
- **Embedded** — Flex child that fills available height and width inside a
  parent layout (for example an information card or split pane).

```tsx
<ChartContainer
  config={chartConfig}
  presentation="embedded"
  title="Embedded throughput chart"
>
  {/* Recharts children */}
</ChartContainer>
```

Both presentations set `data-chart-presentation` on the root element for
testing and layout hooks.

## Tooltip and legend content

`ChartTooltip` is the Recharts `Tooltip` export. Pass `ChartTooltipContent` as
the `content` render prop so labels and swatches resolve from `ChartConfig`:

```tsx
<ChartTooltip content={<ChartTooltipContent />} />
```

`ChartLegend` is the Recharts `Legend` export. For a compact footer legend
outside the Recharts tree, render `ChartLegendContent` in `ChartContainer`'s
`footer` prop and supply `payload` yourself:

```tsx
<ChartContainer
  config={chartConfig}
  footer={
    <ChartLegendContent
      payload={[
        {
          color: chartConfig.alpha.color,
          dataKey: "alpha",
          type: "line",
          value: chartConfig.alpha.label,
        },
      ]}
    />
  }
  title="Chart with footer legend"
>
  {/* Recharts children */}
</ChartContainer>
```

Optional `ChartContainer` props:

| Prop | Purpose |
| ---- | ------- |
| `footer` | Content below the chart surface (typical home for `ChartLegendContent`) |
| `overlay` | Non-interactive overlay inside the chart region |
| `interactionAttributes` | Extra attributes on the chart root (for example drag-to-zoom handlers owned by the host) |
| `rootAttributes` | Additional `data-*` or `aria-*` attributes on the chart root |
| `className` / `style` | Host styling overrides |

## Caller-owned legend state

Legend toggles are optional. When `onToggleSeries` is provided,
`ChartLegendContent` renders keyboard-focusable buttons with `aria-pressed`
reflecting visibility. The package does not store hidden-series state; the host
owns a `Set<string>` (or equivalent) and updates it in the toggle callback.

```tsx
import { useState } from "react";

function InteractiveLegendChart() {
  const [hiddenSeries, setHiddenSeries] = useState<ReadonlySet<string>>(
    () => new Set(),
  );

  const handleToggleSeries = (key: string) => {
    setHiddenSeries((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  return (
    <ChartContainer
      config={chartConfig}
      footer={
        <ChartLegendContent
          getToggleLabel={(label, hidden) =>
            hidden ? `Show ${label}` : `Hide ${label}`
          }
          hiddenSeries={hiddenSeries}
          onToggleSeries={handleToggleSeries}
          payload={legendPayload}
        />
      }
      title="Interactive legend chart"
    >
      <LineChart data={chartData}>
        {!hiddenSeries.has("alpha") ? (
          <Line dataKey="alpha" stroke="var(--color-alpha)" type="monotone" />
        ) : null}
        {!hiddenSeries.has("beta") ? (
          <Line dataKey="beta" stroke="var(--color-beta)" type="monotone" />
        ) : null}
      </LineChart>
    </ChartContainer>
  );
}
```

Filter Recharts series in the host based on `hiddenSeries`. Use
`getToggleLabel` for accessible toggle names when the default label is not
sufficient.

## Chart state panels

Use `ChartStatePanel` when data is not ready for a chart. The `status` prop is
one of `"loading"`, `"empty"`, `"error"`, or `"success"`. Titles,
descriptions, and optional actions are caller-supplied; the package does not
ship product copy.

| Prop | Purpose |
| ---- | ------- |
| `status` | Panel state (`loading`, `empty`, `error`, `success`) |
| `title` | Heading text (`<h3>`) |
| `description` | Supporting copy |
| `action` | Optional retry or navigation control |
| `presentation` | `"standalone"` (dashed bordered shell) or `"embedded"` (fits inside a parent card) |

Accessibility:

- `error` uses `role="alert"` and `aria-live="assertive"`.
- Other states use `role="status"` and `aria-live="polite"`.
- `loading` sets `aria-busy` and shows a skeleton without relying on color
  alone.

### Loading

```tsx
<ChartStatePanel
  description="Waiting for chart data to load."
  status="loading"
  title="Loading chart data"
/>
```

### Empty

```tsx
<ChartStatePanel
  description="No data points are available for this range."
  status="empty"
  title="No chart data"
/>
```

### Error

```tsx
<ChartStatePanel
  action={<button type="button">Retry chart load</button>}
  description="The chart request failed. Try again later."
  status="error"
  title="Unable to load chart"
/>
```

### Success

```tsx
<ChartStatePanel
  description="The chart finished loading successfully."
  status="success"
  title="Chart ready"
/>
```

### Embedded state in a narrow layout

```tsx
<div className="w-64 rounded-2xl border border-outline p-4">
  <ChartStatePanel
    description="No data points are available for this range."
    presentation="embedded"
    status="empty"
    title="No chart data"
  />
</div>
```

Render a state panel instead of `ChartContainer` when the host determines data
is loading, empty, invalid, or failed. Do not embed domain fetching logic inside
the chart package.

## Host application responsibilities

The chart package is presentation-only. Host applications own:

| Responsibility | Examples |
| -------------- | -------- |
| Data fetching | API calls, caching, refresh, and error handling |
| Domain series generation | Aggregating work outcomes, time ranges, and metric keys into `chartData` rows |
| Localized copy | Titles, descriptions, axis labels, and toggle labels passed as props |
| Semantic styling | Mapping domain roles (queued, completed, failed, and so on) to colors and Recharts class names before building `ChartConfig` |

The dashboard work-outcome feature keeps `chart-contract.ts` and
`buildWorkChartData` for semantic colors and deterministic series mapping, then
imports generic primitives from `@you-agent-factory/components/charts` for
rendering. Follow the same boundary in other apps: prepare config and data in
feature code, render with package chart components.

## Storybook and tests

Domain-neutral Storybook examples live under `src/charts/*.stories.tsx`.
Package tests in `src/charts/chart.test.tsx` and
`chart-state-panel.test.tsx` render charts and state panels without dashboard
providers or fixtures. See those files for copy-paste-friendly neutral data
(`chart-story-fixtures.ts`).
