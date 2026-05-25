import { expect, within } from "storybook/test";

import { DashboardActionButton } from "./dashboard-action-button";
import { DashboardActionRow } from "./dashboard-action-row";
import { DashboardStatusPill } from "./dashboard-status-pill";

function DashboardActionControlsShowcase() {
  return (
    <div className="grid gap-6 p-6">
      <section aria-labelledby="dashboard-action-button-variants" className="grid gap-3">
        <h2
          className="text-sm font-semibold text-af-text"
          id="dashboard-action-button-variants"
        >
          Dashboard action button variants
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <DashboardActionButton aria-label="Export factory" iconOnly type="button">
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
          <DashboardActionButton type="button">Save changes</DashboardActionButton>
          <DashboardActionButton type="button">
            <svg aria-hidden="true" fill="none" viewBox="0 0 16 16">
              <path
                d="M3.5 8h9m-4.5-4.5L12.5 8 8 12.5"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
              />
            </svg>
            <span>Enter editor</span>
          </DashboardActionButton>
        </div>
      </section>

      <section aria-labelledby="dashboard-action-button-states" className="grid gap-3">
        <h2
          className="text-sm font-semibold text-af-text"
          id="dashboard-action-button-states"
        >
          Dashboard action button states
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <DashboardActionButton tone="destructive" type="button">
            Remove widget
          </DashboardActionButton>
          <DashboardActionButton aria-pressed={true} tone="secondary" type="button">
            Connect
          </DashboardActionButton>
          <DashboardActionButton disabled type="button">
            Save unavailable
          </DashboardActionButton>
          <DashboardActionButton executing type="button">
            Saving widget
          </DashboardActionButton>
        </div>
      </section>

      <section aria-labelledby="dashboard-status-pill-states" className="grid gap-3">
        <h2
          className="text-sm font-semibold text-af-text"
          id="dashboard-status-pill-states"
        >
          Dashboard status pill states
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <DashboardStatusPill tone="neutral">Observe mode</DashboardStatusPill>
          <DashboardStatusPill role="status" tone="active">
            Active stream
          </DashboardStatusPill>
          <DashboardStatusPill tone="warning">Draft changes pending</DashboardStatusPill>
          <DashboardStatusPill tone="danger">Editor unavailable</DashboardStatusPill>
        </div>
      </section>

      <section aria-labelledby="dashboard-action-row-example" className="grid gap-3">
        <h2
          className="text-sm font-semibold text-af-text"
          id="dashboard-action-row-example"
        >
          Shared dashboard action row
        </h2>
        <DashboardActionRow
          actions={
            <>
              <DashboardActionButton type="button">Discard</DashboardActionButton>
              <DashboardActionButton executing type="button">
                Saving draft
              </DashboardActionButton>
            </>
          }
          aria-label="Shared dashboard action row example"
          statuses={
            <DashboardStatusPill role="status" tone="warning">
              Draft changes pending
            </DashboardStatusPill>
          }
        />
      </section>
    </div>
  );
}

export default {
  title: "Agent Factory/UI/Dashboard Action Controls",
  tags: ["test"],
};

export const SharedDashboardActionControls = {
  render: () => <DashboardActionControlsShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const actionRow = canvas.getByLabelText("Shared dashboard action row example");
    const sections = actionRow.querySelectorAll(
      "[data-dashboard-action-row-section]",
    );

    await expect(
      canvas.getByRole("button", { name: "Export factory" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Save changes" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Enter editor" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Remove widget" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Connect" }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(
      canvas.getByRole("button", { name: "Save unavailable" }),
    ).toBeDisabled();
    await expect(
      canvas.getByRole("button", { name: "Saving widget" }),
    ).toHaveAttribute("aria-busy", "true");
    await expect(canvas.getByText("Active stream")).toBeVisible();
    await expect(canvas.getAllByText("Draft changes pending")).toHaveLength(2);
    await expect(canvas.getByText("Editor unavailable")).toBeVisible();
    await expect(sections).toHaveLength(2);
    await expect(
      sections[0]?.getAttribute("data-dashboard-action-row-section"),
    ).toBe("statuses");
    await expect(
      sections[1]?.getAttribute("data-dashboard-action-row-section"),
    ).toBe("actions");
  },
};
