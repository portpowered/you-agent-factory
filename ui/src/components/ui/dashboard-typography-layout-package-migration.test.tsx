import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import {
  ActionRow as PackageActionRow,
  Code as PackageCode,
  DescriptionList as PackageDescriptionList,
  Heading as PackageHeading,
  Label as PackageLabel,
  SurfacePanel as PackageSurfacePanel,
  Text as PackageText,
  surfacePanelVariants as packageSurfacePanelVariants,
} from "@you-agent-factory/components";

import {
  TrendSummaryGrid,
  TrendSummaryMetric,
} from "../../features/work-outcome/components/trend-summary";
import { AlertPanel } from "./alert-panel";
import { DashboardActionButton } from "./dashboard-action-button";
import { DashboardActionRow } from "./dashboard-action-row";
import { DashboardDescriptionList } from "./dashboard-description-list";
import { DashboardStatusPill } from "./dashboard-status-pill";
import {
  DashboardCode,
  DashboardHeading,
  DashboardLabel,
  DashboardText,
} from "./dashboard-typography-components";
import { FormDescription, FormError, FormWarning } from "./form-field";
import {
  ActionRow,
  Code,
  DescriptionList,
  Heading,
  Label,
  SurfacePanel,
  surfacePanelVariants,
  Text,
} from "./index";

describe("dashboard typography layout package migration", () => {
  it("re-exports package typography and layout primitives from dashboard UI entrypoints", () => {
    expect(Text).toBe(PackageText);
    expect(Heading).toBe(PackageHeading);
    expect(Label).toBe(PackageLabel);
    expect(Code).toBe(PackageCode);
    expect(ActionRow).toBe(PackageActionRow);
    expect(DescriptionList).toBe(PackageDescriptionList);
    expect(SurfacePanel).toBe(PackageSurfacePanel);
    expect(surfacePanelVariants).toBe(packageSurfacePanelVariants);
    expect(DashboardText).toBe(PackageText);
    expect(DashboardHeading).toBe(PackageHeading);
    expect(DashboardLabel).toBe(PackageLabel);
    expect(DashboardCode).toBe(PackageCode);
    expect(DashboardActionRow).toBe(PackageActionRow);
    expect(DashboardDescriptionList).toBe(PackageDescriptionList);
  });

  it("renders representative typography and dense text through dashboard form surfaces", () => {
    render(
      <>
        <Text variant="dense">Dense runtime metadata</Text>
        <FormDescription variant="body">
          Host-owned supporting copy for a migrated field.
        </FormDescription>
      </>,
    );

    expect(screen.getByText("Dense runtime metadata").className).toContain(
      "af-dense-body-text",
    );
    expect(
      screen.getByText("Host-owned supporting copy for a migrated field.")
        .className,
    ).toContain("text-body-medium");
  });

  it("renders representative surface panel layout through dashboard shell primitives", () => {
    render(
      <SurfacePanel
        aria-label="Runtime summary panel"
        padding="compact"
        radius="lg"
        surface="low"
      >
        <Heading as="h3" level="section">
          Session summary
        </Heading>
        <Text>Host-owned panel body copy.</Text>
      </SurfacePanel>,
    );

    const panel = screen.getByLabelText("Runtime summary panel");
    expect(panel.className).toContain("bg-surface-container-low");
    expect(panel.className).toContain("rounded-lg");
    expect(panel.className).toContain("p-2");
    expect(
      screen.getByRole("heading", { level: 3, name: "Session summary" })
        .className,
    ).toContain("af-section-heading");
    expect(screen.getByText("Host-owned panel body copy.").className).toContain(
      "af-body-text",
    );
  });

  it("preserves action-row wrapping semantics for mixed status and action clusters", () => {
    const { container } = render(
      <div className="max-w-xs">
        <ActionRow
          actions={
            <>
              <DashboardActionButton type="button">
                Discard
              </DashboardActionButton>
              <DashboardActionButton type="button">
                Save draft
              </DashboardActionButton>
              <DashboardActionButton type="button">
                Publish changes
              </DashboardActionButton>
            </>
          }
          statuses={
            <DashboardStatusPill role="status" tone="warning">
              Unsaved changes
            </DashboardStatusPill>
          }
        />
      </div>,
    );

    const actionRow = container.querySelector("[data-action-row-section]");
    expect(actionRow?.parentElement?.className).toContain("flex-wrap");
    expect(actionRow?.className).toContain("min-w-0");
    expect(actionRow?.className).toContain("flex-wrap");
    expect(screen.getByRole("status").textContent).toBe("Unsaved changes");
    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual(["Discard", "Save draft", "Publish changes"]);
  });

  it("renders representative description-list consumer layout through trend summary", () => {
    render(
      <TrendSummaryGrid aria-label="Outcome trend summary">
        <TrendSummaryMetric label="Failed in range" value={3} />
        <TrendSummaryMetric
          label="Long host label for throughput comparison"
          value="—"
        />
      </TrendSummaryGrid>,
    );

    const summary = screen.getByLabelText("Outcome trend summary");
    expect(summary.tagName).toBe("DL");
    expect(summary.className).toContain("md:grid-cols-3");
    expect(screen.getByText("Failed in range").tagName).toBe("DT");
    expect(screen.getByText("3").tagName).toBe("DD");
    expect(screen.getByText("—").textContent).toBe("—");
    expect(
      screen
        .getByText("Long host label for throughput comparison")
        .closest("div")?.className,
    ).toContain("bg-surface-container-low");
  });

  it("keeps loading, empty, error, and success copy host-owned in migrated surfaces", () => {
    render(
      <>
        <FormError role="alert">Host validation message</FormError>
        <FormWarning role="status">Host warning message</FormWarning>
        <AlertPanel tone="success">Host success message</AlertPanel>
        <AlertPanel variant="empty">Host empty-state message</AlertPanel>
      </>,
    );

    expect(screen.getByRole("alert").textContent).toBe(
      "Host validation message",
    );
    expect(screen.getByRole("status").textContent).toBe("Host warning message");
    expect(screen.getByText("Host success message").className).toContain(
      "bg-success-container",
    );
    expect(screen.getByText("Host empty-state message").className).toContain(
      "border-dashed",
    );
  });
});
