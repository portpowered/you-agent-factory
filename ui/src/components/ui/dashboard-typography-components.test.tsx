import { render, screen } from "@testing-library/react";

import {
  DashboardCode,
  DashboardHeading,
  DashboardLabel,
  DashboardText,
} from "./dashboard-typography-components";

describe("dashboard typography components", () => {
  it("renders page and section heading roles", () => {
    render(
      <>
        <DashboardHeading level="page">Factory dashboard</DashboardHeading>
        <DashboardHeading as="h4">Runtime details</DashboardHeading>
      </>,
    );

    expect(
      screen.getByRole("heading", { level: 1, name: "Factory dashboard" })
        .className,
    ).toContain("af-dashboard-page-heading");
    expect(
      screen.getByRole("heading", { level: 4, name: "Runtime details" })
        .className,
    ).toContain("af-dashboard-section-heading");
  });

  it("renders body and supporting text roles on configurable elements", () => {
    render(
      <>
        <DashboardText>Body copy</DashboardText>
        <DashboardText as="span" variant="supporting">
          Supporting copy
        </DashboardText>
      </>,
    );

    expect(screen.getByText("Body copy").tagName).toBe("P");
    expect(screen.getByText("Body copy").className).toContain(
      "af-dashboard-body-text",
    );
    expect(screen.getByText("Supporting copy").tagName).toBe("SPAN");
    expect(screen.getByText("Supporting copy").className).toContain(
      "af-dashboard-supporting-text",
    );
  });

  it("renders label and code typography roles", () => {
    render(
      <>
        <DashboardLabel as="dt">Dispatch ID</DashboardLabel>
        <DashboardCode size="supporting">dispatch-1</DashboardCode>
      </>,
    );

    expect(screen.getByText("Dispatch ID").tagName).toBe("DT");
    expect(screen.getByText("Dispatch ID").className).toContain(
      "af-dashboard-supporting-label",
    );
    expect(screen.getByText("dispatch-1").className).toContain(
      "af-dashboard-supporting-code",
    );
  });
});
