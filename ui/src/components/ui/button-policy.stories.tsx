import { expect, within } from "storybook/test";

import { Button } from "./button";
import { DashboardActionButton } from "./dashboard-action-button";
import { DashboardActionRow } from "./dashboard-action-row";
import { DashboardStatusPill } from "./dashboard-status-pill";

const policyCards = [
  {
    approved:
      "Use the shared Button primitive for customer-facing submit, confirm, cancel, import, export, and comparable form or dialog actions.",
    id: "ordinary-action-lane",
    title: "Ordinary actions use Button",
    when:
      "Forms, dialogs, submit rows, empty states, and other standard action groups.",
  },
  {
    approved:
      "Use DashboardActionButton or DashboardActionRow for compact dashboard headers, detail-card chrome, and factory graph editor toolbars.",
    id: "dashboard-action-lane",
    title: "Compact dashboard actions use DashboardActionButton",
    when:
      "Dashboard headers, inline action rows, widget chrome, graph-editor controls, and similar dense control surfaces.",
  },
  {
    approved:
      "Only keep raw semantic-button ownership behind a dedicated wrapper or narrow documented exception when the surface is structurally different from an ordinary action button.",
    id: "semantic-exception-lane",
    title: "Semantic-button exceptions stay narrow",
    when:
      "Tabs, disclosure shells, selectable rows, graph nodes, and other stateful interaction shells.",
  },
];

const semanticExceptions = [
  {
    category: "Tabs and segmented controls",
    guidance:
      "Preserve tablist or pressed-state semantics through a dedicated tab owner instead of swapping in Button.",
  },
  {
    category: "Disclosure triggers",
    guidance:
      "Keep wrapper ownership where the trigger controls expanded content and needs disclosure-specific semantics.",
  },
  {
    category: "Selectable rows and cards",
    guidance:
      "Keep listbox, option, or selection-shell semantics explicit so selection state is not reduced to a generic action button.",
  },
  {
    category: "Graph nodes and canvas tools",
    guidance:
      "Use graph-specific wrappers when the control participates in drag, placement, or canvas navigation semantics.",
  },
];

function PolicyCard({
  approved,
  id,
  title,
  when,
}: {
  approved: string;
  id: string;
  title: string;
  when: string;
}) {
  return (
    <section
      aria-labelledby={id}
      className="grid gap-3 rounded-2xl border border-af-border bg-af-surface-raised p-4"
    >
      <div className="grid gap-1">
        <h2 className="m-0 text-lg font-semibold text-af-text" id={id}>
          {title}
        </h2>
        <p className="m-0 text-sm text-af-text-muted">{when}</p>
      </div>
      <p className="m-0 text-sm leading-6 text-af-text">{approved}</p>
    </section>
  );
}

function ButtonPolicyShowcase() {
  return (
    <div className="grid gap-6 rounded-3xl border border-af-border bg-af-surface-subtle p-6 text-af-text">
      <section className="grid gap-2">
        <div>
          <h1 className="m-0 font-display text-3xl tracking-[-0.03em]">
            Website button policy
          </h1>
          <p className="m-0 pt-2 text-sm leading-6 text-af-text-muted">
            Production UI under <code>ui/src</code> should stay in one of three lanes:
            standard actions use <code>Button</code>, compact dashboard actions use
            <code> DashboardActionButton</code>, and structurally different semantic
            controls stay behind narrow wrapper-owned exceptions.
          </p>
        </div>
      </section>

      <section className="grid gap-4 lg:grid-cols-3">
        {policyCards.map((card) => (
          <PolicyCard key={card.id} {...card} />
        ))}
      </section>

      <section aria-labelledby="ordinary-button-patterns" className="grid gap-3">
        <div className="grid gap-1">
          <h2 className="m-0 text-xl font-semibold text-af-text" id="ordinary-button-patterns">
            Approved ordinary Button patterns
          </h2>
          <p className="m-0 text-sm text-af-text-muted">
            Reuse shared tones and sizes for primary, destructive, secondary, icon-only,
            and lightweight actions instead of owning local button styling.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button type="button">Primary action</Button>
          <Button tone="secondary" type="button">
            Secondary action
          </Button>
          <Button tone="outline" type="button">
            Outline action
          </Button>
          <Button tone="ghost" type="button">
            Ghost action
          </Button>
          <Button tone="destructive" type="button">
            Delete factory
          </Button>
          <Button aria-label="Refresh jobs" size="icon" tone="outline" type="button">
            <svg aria-hidden="true" fill="none" viewBox="0 0 16 16">
              <path
                d="M13 8a5 5 0 1 1-1.46-3.54M13 3.5V7h-3.5"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
              />
            </svg>
          </Button>
        </div>
      </section>

      <section aria-labelledby="dashboard-button-patterns" className="grid gap-3">
        <div className="grid gap-1">
          <h2 className="m-0 text-xl font-semibold text-af-text" id="dashboard-button-patterns">
            Approved compact dashboard patterns
          </h2>
          <p className="m-0 text-sm text-af-text-muted">
            Compact control rows keep shared spacing, focus treatment, disabled state, and
            executing feedback through the dashboard action family.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <DashboardActionButton aria-label="Export dashboard" iconOnly type="button">
            <svg aria-hidden="true" fill="none" viewBox="0 0 16 16">
              <path
                d="M8 2v8m0 0 3-3m-3 3L5 7M3 12.5h10"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
              />
            </svg>
          </DashboardActionButton>
          <DashboardActionButton type="button">Save draft</DashboardActionButton>
          <DashboardActionButton aria-pressed={true} tone="secondary" type="button">
            Connect nodes
          </DashboardActionButton>
          <DashboardActionButton executing type="button">
            Syncing graph
          </DashboardActionButton>
        </div>
        <DashboardActionRow
          actions={
            <>
              <DashboardActionButton type="button">Discard</DashboardActionButton>
              <DashboardActionButton executing type="button">
                Publishing
              </DashboardActionButton>
            </>
          }
          aria-label="Dashboard action row policy example"
          statuses={
            <>
              <DashboardStatusPill role="status" tone="warning">
                Draft changes pending
              </DashboardStatusPill>
              <DashboardStatusPill tone="neutral">Observe mode</DashboardStatusPill>
            </>
          }
        />
      </section>

      <section aria-labelledby="semantic-exception-patterns" className="grid gap-3">
        <div className="grid gap-1">
          <h2
            className="m-0 text-xl font-semibold text-af-text"
            id="semantic-exception-patterns"
          >
            Narrow semantic-button exception categories
          </h2>
          <p className="m-0 text-sm text-af-text-muted">
            These surfaces may keep custom ownership when they need specialized semantics,
            but they should not become a back door for ordinary action-button styling.
          </p>
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          {semanticExceptions.map((exception) => (
            <article
              className="grid gap-1 rounded-2xl border border-af-border bg-af-surface-raised p-4"
              key={exception.category}
            >
              <h3 className="m-0 text-sm font-semibold text-af-text">{exception.category}</h3>
              <p className="m-0 text-sm leading-6 text-af-text-muted">
                {exception.guidance}
              </p>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}

export default {
  title: "Agent Factory/UI/Button Policy",
  component: ButtonPolicyShowcase,
  tags: ["test"],
};

export const Default = {
  render: () => <ButtonPolicyShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("Website button policy")).toBeVisible();
    await expect(canvas.getByText("Ordinary actions use Button")).toBeVisible();
    await expect(
      canvas.getByText("Compact dashboard actions use DashboardActionButton"),
    ).toBeVisible();
    await expect(
      canvas.getByText("Semantic-button exceptions stay narrow"),
    ).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Primary action" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Delete factory" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Refresh jobs" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Export dashboard" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Syncing graph" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
    await expect(
      canvas.getByLabelText("Dashboard action row policy example"),
    ).toBeVisible();
    await expect(canvas.getByText("Tabs and segmented controls")).toBeVisible();
    await expect(canvas.getByText("Disclosure triggers")).toBeVisible();
  },
};
