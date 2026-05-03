import { useState } from "react";
import { Area, AreaChart, CartesianGrid, XAxis } from "recharts";

import {
  Button,
  Calendar,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DataTable,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
  Select,
  Skeleton,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Textarea,
} from ".";

const chartData = [
  { day: "Mon", completed: 2, failed: 1 },
  { day: "Tue", completed: 4, failed: 1 },
  { day: "Wed", completed: 3, failed: 2 },
  { day: "Thu", completed: 6, failed: 1 },
  { day: "Fri", completed: 5, failed: 0 },
];

const chartConfig = {
  completed: { color: "var(--color-af-chart-completed)", label: "Completed" },
  failed: { color: "var(--color-af-chart-failed)", label: "Failed" },
};

const showcaseDispatchRows = [
  {
    dispatch: "dispatch-review-1",
    duration: "420ms",
    status: "ACCEPTED",
  },
  {
    dispatch: "dispatch-review-2",
    duration: "1.2s",
    status: "FAILED",
  },
];

export interface UIFoundationShowcaseProps {
  includeResizable?: boolean;
}

export function UIFoundationShowcase({ includeResizable = true }: UIFoundationShowcaseProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [collapseOpen, setCollapseOpen] = useState(true);
  const requestNameID = "ui-foundation-showcase-request-name";
  const requestTextID = "ui-foundation-showcase-request-text";
  const workTypeID = "ui-foundation-showcase-work-type";

  return (
    <div className="grid gap-6 rounded-[1.8rem] border border-af-overlay/10 bg-af-surface/52 p-6 text-af-ink">
      <section className="grid gap-3">
        <div>
          <h2 className="m-0 font-display text-3xl tracking-[-0.03em]">Shared UI primitives</h2>
          <p className="m-0 pt-2 text-sm text-af-ink/66">
            Shared button, field, dialog, chart, table, skeleton, collapsible, calendar, and resizable building blocks.
          </p>
        </div>

        <div className="flex flex-wrap gap-3">
          <Button>Primary action</Button>
          <Button tone="secondary">Secondary</Button>
          <Button tone="outline">Outline</Button>
          <Button disabled>Disabled action</Button>
        </div>
      </section>

      <section className="grid gap-3 md:grid-cols-2">
        <div className="grid gap-2">
          <label className="text-xs font-bold uppercase tracking-[0.08em] text-af-ink/58" htmlFor={requestNameID}>
            Request name
          </label>
          <Input id={requestNameID} placeholder="Name this request" />
        </div>

        <div className="grid gap-2">
          <label className="text-xs font-bold uppercase tracking-[0.08em] text-af-ink/58" htmlFor={workTypeID}>
            Work type
          </label>
          <Select defaultValue="story" id={workTypeID}>
            <option value="story">story</option>
            <option value="task">task</option>
          </Select>
        </div>

        <div className="grid gap-2 md:col-span-2">
          <label className="text-xs font-bold uppercase tracking-[0.08em] text-af-ink/58" htmlFor={requestTextID}>
            Request text
          </label>
          <Textarea id={requestTextID} placeholder="Describe the work to run" />
        </div>
      </section>

      <section className="grid gap-3 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,0.7fr)]">
        <div className="grid gap-3">
          <ChartContainer config={chartConfig} title="Primitive chart showcase">
            <AreaChart data={chartData} margin={{ left: 8, right: 8, top: 12 }}>
              <CartesianGrid stroke="rgb(from var(--color-af-overlay) r g b / 0.12)" vertical={false} />
              <XAxis axisLine={false} dataKey="day" tickLine={false} tick={{ fill: "rgb(from var(--color-af-ink) r g b / 0.58)", fontSize: 12 }} />
              <ChartTooltip
                content={(props) => <ChartTooltipContent {...props} />}
                cursor={{ stroke: "rgb(from var(--color-af-overlay) r g b / 0.16)" }}
              />
              <ChartLegend content={<ChartLegendContent />} />
              <Area
                dataKey="completed"
                fill="rgb(from var(--color-af-chart-completed) r g b / 0.2)"
                fillOpacity={1}
                stroke="var(--color-af-chart-completed)"
                strokeWidth={2}
                type="monotone"
              />
              <Area
                dataKey="failed"
                fill="rgb(from var(--color-af-chart-failed) r g b / 0.18)"
                fillOpacity={1}
                stroke="var(--color-af-chart-failed)"
                strokeWidth={2}
                type="monotone"
              />
            </AreaChart>
          </ChartContainer>

          <Table>
            <TableCaption>Primitive table foundation for trace and detail surfaces.</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>Dispatch</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Duration</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>dispatch-review-1</TableCell>
                <TableCell>ACCEPTED</TableCell>
                <TableCell>420ms</TableCell>
              </TableRow>
              <TableRow data-state="selected">
                <TableCell>dispatch-review-2</TableCell>
                <TableCell>FAILED</TableCell>
                <TableCell>1.2s</TableCell>
              </TableRow>
            </TableBody>
          </Table>

          <DataTable
            ariaLabel="Primitive data table showcase"
            caption="Reusable data table helper for dashboard detail grids."
            columns={[
              { cell: (row) => row.dispatch, header: "Dispatch", id: "dispatch" },
              { cell: (row) => row.status, header: "Status", id: "status" },
              { cell: (row) => row.duration, header: "Duration", id: "duration" },
            ]}
            data={showcaseDispatchRows}
            getRowKey={(row) => row.dispatch}
            rowClassName={(row) => (row.status === "FAILED" ? "data-[state=selected]:bg-af-accent/10" : undefined)}
          />
        </div>

        <div className="grid gap-3">
          <div className="grid gap-3 rounded-2xl border border-af-overlay/10 bg-af-surface/60 p-4">
            <div className="grid gap-2">
              <p className="m-0 text-xs font-bold uppercase tracking-[0.08em] text-af-ink/58">
                Loading
              </p>
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-24 w-full" />
            </div>

            <Collapsible onOpenChange={setCollapseOpen} open={collapseOpen}>
              <div className="flex items-center justify-between gap-3">
                <p className="m-0 text-sm font-semibold">Collapsible section</p>
                <CollapsibleTrigger asChild>
                  <Button aria-expanded={collapseOpen} size="sm" tone="ghost">
                    {collapseOpen ? "Collapse" : "Expand"}
                  </Button>
                </CollapsibleTrigger>
              </div>
              <CollapsibleContent className="pt-3">
                <p className="m-0 text-sm leading-6 text-af-ink/72">
                  Shared disclosure state is ready for section toggles and drill-down controls.
                </p>
              </CollapsibleContent>
            </Collapsible>

            <div className="flex flex-wrap gap-3">
              <Button onClick={() => setDialogOpen(true)} tone="outline">
                Open dialog
              </Button>
              <Dialog onOpenChange={setDialogOpen} open={dialogOpen}>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Export factory</DialogTitle>
                    <DialogDescription>
                      Shared dialog chrome for export and confirmation flows.
                    </DialogDescription>
                  </DialogHeader>
                  <div className="grid gap-3">
                    <Input aria-label="Factory name" defaultValue="demo-factory" />
                    <Textarea aria-label="Export notes" defaultValue="Ready for downstream review." />
                  </div>
                  <DialogFooter>
                    <Button onClick={() => setDialogOpen(false)} tone="ghost">
                      Cancel
                    </Button>
                    <Button>Confirm export</Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>
          </div>

          <Calendar
            aria-label="Primitive calendar showcase"
            defaultMonth={new Date("2026-05-01T00:00:00Z")}
            mode="single"
            selected={new Date("2026-05-14T00:00:00Z")}
          />
        </div>
      </section>

      {includeResizable ? (
        <section className="grid gap-3">
          <p className="m-0 text-xs font-bold uppercase tracking-[0.08em] text-af-ink/58">
            Resizable panels
          </p>
          <div className="h-44 overflow-hidden rounded-2xl border border-af-overlay/10 bg-af-surface/56">
            <ResizablePanelGroup orientation="horizontal">
              <ResizablePanel defaultSize={45} minSize={30}>
                <div className="flex h-full items-center justify-center bg-af-canvas/52 px-4 text-sm text-af-ink/72">
                  Sidebar panel
                </div>
              </ResizablePanel>
              <ResizableHandle withHandle />
              <ResizablePanel defaultSize={55} minSize={30}>
                <div className="flex h-full items-center justify-center px-4 text-sm text-af-ink/72">
                  Detail panel
                </div>
              </ResizablePanel>
            </ResizablePanelGroup>
          </div>
        </section>
      ) : null}
    </div>
  );
}
