import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  DispatchDetailList,
  DispatchDetailSection,
} from "./dispatch-detail-sections";

describe("dispatch detail sections", () => {
  it("renders a standard detail section heading and region", () => {
    render(
      <DispatchDetailSection title="Failure details">
        <p>Provider timed out.</p>
      </DispatchDetailSection>,
    );

    const section = screen.getByRole("region", { name: "Failure details" });
    const heading = screen.getByRole("heading", {
      level: 4,
      name: "Failure details",
    });

    expect(section.className).toContain("border-t");
    expect(section.className).toContain("pt-4");
    expect(heading.className).toContain("m-0");
  });

  it("renders text, code, and link detail entries", () => {
    render(
      <DispatchDetailList
        entries={[
          { label: "Reason", value: "provider_timeout" },
          { code: true, label: "Class", value: "TimeoutError" },
          {
            href: "/logs/session.log",
            label: "Logs",
            title: "Session logs",
            value: "Open logs",
          },
          { label: "Empty", value: undefined },
        ]}
      />,
    );

    expect(screen.getByText("Reason")).toBeTruthy();
    expect(screen.getByText("TimeoutError").tagName).toBe("CODE");

    const link = screen.getByRole("link", { name: "Open logs" });

    expect(link.getAttribute("href")).toBe("/logs/session.log");
    expect(link.className).toContain("border-outline");
    expect(screen.queryByText("Empty")).toBeNull();
  });

  it("omits the list when no entries have values", () => {
    const { container } = render(
      <DispatchDetailList entries={[{ label: "Reason" }]} />,
    );

    expect(container.firstChild).toBeNull();
  });
});
