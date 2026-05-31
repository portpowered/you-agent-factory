import "@testing-library/jest-dom/vitest";
import { within } from "@testing-library/react";
import { expect } from "vitest";

import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../components/ui/dashboard-typography";

export function expectSingleWorkOutcomeCardHeader(
  card: HTMLElement,
  {
    cardTitle,
    cardRegionLabel,
    headerActionLabel,
  }: {
    cardTitle: string;
    cardRegionLabel: string;
    headerActionLabel?: string;
  },
): void {
  const headers = card.querySelectorAll("header");
  expect(headers).toHaveLength(1);

  const cardHeader = headers[0] as HTMLElement;
  expect(cardHeader.getAttribute("data-bento-drag-handle")).toBe("true");

  const titleHeading = within(cardHeader).getByRole("heading", {
    level: 3,
    name: cardTitle,
  });
  expect(titleHeading.className).toContain(DASHBOARD_SECTION_HEADING_CLASS);

  const chartRegion = within(card).getByLabelText(cardRegionLabel);
  expect(
    within(chartRegion).queryByRole("heading", {
      level: 3,
      name: cardTitle,
    }),
  ).toBeNull();
  expect(within(chartRegion).queryByText(cardTitle)).toBeNull();

  if (headerActionLabel) {
    const headerTools = cardHeader.querySelector(
      "[class*='shrink-0'][class*='items-center']",
    );
    const headerAction = within(card).getByRole("button", {
      name: headerActionLabel,
    });
    expect(headerTools).toBeTruthy();
    expect(headerTools?.contains(headerAction)).toBe(true);
    expect(titleHeading.compareDocumentPosition(headerAction)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  }
}
