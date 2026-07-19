import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";

import { ActiveWorkRows } from "./factory-topology-active-work";
import { createGermanRecordingMessages } from "./testing/factory-recording-messages";

it("uses caller-localized active Work row, duration, and overflow messages", () => {
  render(
    <ActiveWorkRows
      items={Array.from({ length: 5 }, (_, index) => ({
        durationTicks: 4,
        id: `work-${index + 1}`,
      }))}
      messages={createGermanRecordingMessages().topology}
    />,
  );

  expect(
    screen.getByRole("group", { name: "5 aktive Arbeitszeilen" }),
  ).toBeVisible();
  expect(screen.getAllByText("4 logische Schritte")).toHaveLength(3);
  expect(screen.getByText("+2 weitere Aufträge")).toBeVisible();
});
