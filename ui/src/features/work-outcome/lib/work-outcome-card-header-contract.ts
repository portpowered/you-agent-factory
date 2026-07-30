import { within } from "@testing-library/react";
import { expect } from "storybook/test";

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
  expect(titleHeading.className).toContain("af-section-heading");

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
    expect(headerTools?.className).toContain("ml-auto");
    expect(headerTools?.className).not.toContain("w-full");
    expect(headerTools?.className).not.toContain("flex-wrap");
    expect(titleHeading.compareDocumentPosition(headerAction)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  }
}

export function expectWorkOutcomeCardFillsChartBody(
  card: HTMLElement,
  cardRegionLabel: string,
): void {
  const body = Array.from(card.children).find(
    (child): child is HTMLElement =>
      child instanceof HTMLElement && child.tagName.toLowerCase() !== "header",
  );
  expect(body).toBeTruthy();
  expect(body?.className).toContain("!h-full");
  expect(body?.className).toContain("!flex-1");
  expect(body?.className).toContain("!overflow-hidden");
  expect(card.querySelector("[data-radix-scroll-area-viewport]")).toBeNull();

  const chartRegion = within(card).getByLabelText(cardRegionLabel);
  expect(chartRegion.className).toContain("h-full");
  expect(chartRegion.className).toContain("flex-1");
  expect(chartRegion.className).toContain("min-h-0");
}
