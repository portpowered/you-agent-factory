import { useState } from "react";
import { Area, AreaChart, CartesianGrid, XAxis } from "recharts";

import { useAppLocale } from "../../i18n";
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
import { getSharedPrimitiveMessages } from "./messages/shared-primitives";

const chartData = [
  { day: "Mon", completed: 2, failed: 1 },
  { day: "Tue", completed: 4, failed: 1 },
  { day: "Wed", completed: 3, failed: 2 },
  { day: "Thu", completed: 6, failed: 1 },
  { day: "Fri", completed: 5, failed: 0 },
];

export interface UIFoundationShowcaseProps {
  includeResizable?: boolean;
  locale?: string | null;
}

export function UIFoundationShowcase({
  includeResizable = true,
  locale: localeOverride,
}: UIFoundationShowcaseProps) {
  const { locale } = useAppLocale(localeOverride);
  const messages = getSharedPrimitiveMessages(locale);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [collapseOpen, setCollapseOpen] = useState(true);
  const requestNameID = "ui-foundation-showcase-request-name";
  const requestTextID = "ui-foundation-showcase-request-text";
  const workTypeID = "ui-foundation-showcase-work-type";
  const chartConfig = {
    completed: {
      color: "var(--color-af-chart-completed)",
      label: messages.chartCompletedLabel,
    },
    failed: {
      color: "var(--color-af-chart-failed)",
      label: messages.chartFailedLabel,
    },
  };
  const showcaseDispatchRows = [
    {
      dispatch: messages.dispatchReviewOneLabel,
      duration: messages.durationShortSample,
      status: messages.dispatchAcceptedSample,
    },
    {
      dispatch: messages.dispatchReviewTwoLabel,
      duration: messages.durationLongSample,
      status: messages.dispatchFailedSample,
    },
  ];

  return (
    <div className="grid gap-6 rounded-2xl border border-af-border bg-af-surface-subtle p-6 text-af-text">
      <section className="grid gap-3">
        <div>
          <h2 className="m-0 font-display text-3xl tracking-[-0.03em]">
            {messages.showcaseTitle}
          </h2>
          <p className="m-0 pt-2 text-sm text-af-text-muted">
            {messages.showcaseDescription}
          </p>
        </div>

        <div className="flex flex-wrap gap-3">
          <Button>{messages.primaryAction}</Button>
          <Button tone="secondary">{messages.secondaryAction}</Button>
          <Button tone="outline">{messages.outlineAction}</Button>
          <Button disabled>{messages.disabledAction}</Button>
        </div>
      </section>

      <section className="grid gap-3 md:grid-cols-2">
        <div className="grid gap-2">
          <label
            className="text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle"
            htmlFor={requestNameID}
          >
            {messages.requestNameLabel}
          </label>
          <Input
            id={requestNameID}
            placeholder={messages.requestNamePlaceholder}
          />
        </div>

        <div className="grid gap-2">
          <label
            className="text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle"
            htmlFor={workTypeID}
          >
            {messages.workTypeLabel}
          </label>
          <Select defaultValue="story" id={workTypeID}>
            <option value="story">{messages.workTypeStoryOption}</option>
            <option value="task">{messages.workTypeTaskOption}</option>
          </Select>
        </div>

        <div className="grid gap-2 md:col-span-2">
          <label
            className="text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle"
            htmlFor={requestTextID}
          >
            {messages.requestTextLabel}
          </label>
          <Textarea
            id={requestTextID}
            placeholder={messages.requestTextPlaceholder}
          />
        </div>
      </section>

      <section className="grid gap-3 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,0.7fr)]">
        <div className="grid gap-3">
          <ChartContainer
            config={chartConfig}
            title={messages.showcaseChartTitle}
          >
            <AreaChart data={chartData} margin={{ left: 8, right: 8, top: 12 }}>
              <CartesianGrid stroke="var(--color-af-border)" vertical={false} />
              <XAxis
                axisLine={false}
                dataKey="day"
                tick={{ fill: "var(--color-af-text-subtle)", fontSize: 12 }}
                tickLine={false}
              />
              <ChartTooltip
                content={(props) => <ChartTooltipContent {...props} />}
                cursor={{ stroke: "var(--color-af-border-strong)" }}
              />
              <ChartLegend content={<ChartLegendContent />} />
              <Area
                dataKey="completed"
                fill="var(--color-af-success-surface)"
                fillOpacity={1}
                stroke="var(--color-af-chart-completed)"
                strokeWidth={2}
                type="monotone"
              />
              <Area
                dataKey="failed"
                fill="var(--color-af-danger-surface)"
                fillOpacity={1}
                stroke="var(--color-af-chart-failed)"
                strokeWidth={2}
                type="monotone"
              />
            </AreaChart>
          </ChartContainer>

          <Table>
            <TableCaption>{messages.tableCaption}</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>{messages.dispatchColumnLabel}</TableHead>
                <TableHead>{messages.statusColumnLabel}</TableHead>
                <TableHead>{messages.durationColumnLabel}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>{messages.dispatchReviewOneLabel}</TableCell>
                <TableCell>{messages.dispatchAcceptedSample}</TableCell>
                <TableCell>{messages.durationShortSample}</TableCell>
              </TableRow>
              <TableRow data-state="selected">
                <TableCell>{messages.dispatchReviewTwoLabel}</TableCell>
                <TableCell>{messages.dispatchFailedSample}</TableCell>
                <TableCell>{messages.durationLongSample}</TableCell>
              </TableRow>
            </TableBody>
          </Table>

          <DataTable
            ariaLabel={messages.dataTableAriaLabel}
            caption={messages.dataTableCaption}
            columns={[
              {
                cell: (row) => row.dispatch,
                header: messages.dispatchColumnLabel,
                id: "dispatch",
              },
              {
                cell: (row) => row.status,
                header: messages.statusColumnLabel,
                id: "status",
              },
              {
                cell: (row) => row.duration,
                header: messages.durationColumnLabel,
                id: "duration",
              },
            ]}
            data={showcaseDispatchRows}
            getRowKey={(row) => row.dispatch}
            rowClassName={(row) =>
              row.status === "FAILED"
                ? "data-[state=selected]:bg-af-danger-surface"
                : undefined
            }
          />
        </div>

        <div className="grid gap-3">
          <div className="grid gap-3 rounded-2xl border border-af-border bg-af-surface-subtle p-4">
            <div className="grid gap-2">
              <p className="m-0 text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle">
                {messages.loadingLabel}
              </p>
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-24 w-full" />
            </div>

            <Collapsible onOpenChange={setCollapseOpen} open={collapseOpen}>
              <div className="flex items-center justify-between gap-3">
                <p className="m-0 text-sm font-semibold">
                  {messages.collapsibleSectionLabel}
                </p>
                <CollapsibleTrigger asChild>
                  <Button aria-expanded={collapseOpen} size="sm" tone="ghost">
                    {collapseOpen
                      ? messages.collapseAction
                      : messages.expandAction}
                  </Button>
                </CollapsibleTrigger>
              </div>
              <CollapsibleContent className="pt-3">
                <p className="m-0 text-sm leading-6 text-af-text-muted">
                  {messages.collapsibleBody}
                </p>
              </CollapsibleContent>
            </Collapsible>

            <div className="flex flex-wrap gap-3">
              <Button onClick={() => setDialogOpen(true)} tone="outline">
                {messages.dialogOpenAction}
              </Button>
              <Dialog onOpenChange={setDialogOpen} open={dialogOpen}>
                <DialogContent locale={locale}>
                  <DialogHeader>
                    <DialogTitle>{messages.dialogTitle}</DialogTitle>
                    <DialogDescription>
                      {messages.dialogDescription}
                    </DialogDescription>
                  </DialogHeader>
                  <div className="grid gap-3">
                    <Input
                      aria-label={messages.dialogFactoryNameLabel}
                      defaultValue="demo-factory"
                    />
                    <Textarea
                      aria-label={messages.dialogExportNotesLabel}
                      defaultValue="Ready for downstream review."
                    />
                  </div>
                  <DialogFooter>
                    <Button onClick={() => setDialogOpen(false)} tone="ghost">
                      {messages.dialogCancelAction}
                    </Button>
                    <Button>{messages.confirmExportAction}</Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>
          </div>

          <Calendar
            aria-label={messages.showcaseCalendarLabel}
            defaultMonth={new Date("2026-05-01T00:00:00Z")}
            mode="single"
            selected={new Date("2026-05-14T00:00:00Z")}
          />
        </div>
      </section>

      {includeResizable ? (
        <section className="grid gap-3">
          <p className="m-0 text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle">
            {messages.resizablePanelsLabel}
          </p>
          <div className="h-44 overflow-hidden rounded-2xl border border-af-border bg-af-surface-subtle">
            <ResizablePanelGroup orientation="horizontal">
              <ResizablePanel defaultSize={45} minSize={30}>
                <div className="flex h-full items-center justify-center bg-af-surface-raised px-4 text-sm text-af-text-muted">
                  {messages.sidebarPanelLabel}
                </div>
              </ResizablePanel>
              <ResizableHandle withHandle />
              <ResizablePanel defaultSize={55} minSize={30}>
                <div className="flex h-full items-center justify-center px-4 text-sm text-af-text-muted">
                  {messages.detailPanelLabel}
                </div>
              </ResizablePanel>
            </ResizablePanelGroup>
          </div>
        </section>
      ) : null}
    </div>
  );
}
