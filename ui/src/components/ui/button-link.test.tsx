import { render, screen } from "@testing-library/react";

import { ButtonLink } from "./button-link";

describe("ButtonLink", () => {
  it("renders a real anchor with shared button variants", () => {
    render(
      <ButtonLink href="/logs/session.log" size="sm" tone="outline">
        Open logs
      </ButtonLink>,
    );

    const link = screen.getByRole("link", { name: "Open logs" });

    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe("/logs/session.log");
    expect(link.className).toContain("border-outline");
    expect(link.className).toContain("min-h-9");
  });

  it("accepts custom link attributes without losing shared styling", () => {
    render(
      <ButtonLink
        className="w-fit"
        href="https://example.com"
        rel="noreferrer"
        target="_blank"
        title="External report"
        tone="secondary"
      >
        View report
      </ButtonLink>,
    );

    const link = screen.getByRole("link", { name: "View report" });

    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noreferrer");
    expect(link.getAttribute("title")).toBe("External report");
    expect(link.className).toContain("bg-surface-container-low");
    expect(link.className).toContain("w-fit");
  });
});
