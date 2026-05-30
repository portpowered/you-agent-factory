import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { WorkContentPartList } from "./work-content-part-list";
import { WorkContentReadOnlyList } from "./work-content-read-only-list";

describe("WorkContentPartList", () => {
  it("renders text parts with authored markdown typography", () => {
    render(
      <WorkContentPartList
        content={[
          {
            text: "## Review notes\n\n- Confirm payload visibility",
            type: "text",
          },
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", { level: 2, name: "Review notes" }),
    ).toBeTruthy();
    expect(screen.getByText("Confirm payload visibility")).toBeTruthy();
  });

  it("renders JSON parts as formatted code blocks", () => {
    render(
      <WorkContentPartList
        content={[
          {
            json: { status: "ready" },
            type: "JSON",
          },
        ]}
      />,
    );

    expect(screen.getByText(/"status": "ready"/)).toBeTruthy();
  });

  it("renders non-text parts with readable fallback descriptions", () => {
    render(
      <WorkContentPartList
        content={[
          {
            contentType: "image/png",
            file: "diagram.png",
            type: "IMAGE",
          },
        ]}
      />,
    );

    expect(screen.getByText("Image: diagram.png")).toBeTruthy();
  });
});

describe("WorkContentReadOnlyList", () => {
  it("renders loading, unavailable, error, and empty status messages", () => {
    const { rerender } = render(
      <WorkContentReadOnlyList payloadStatus="LOADING" />,
    );
    expect(screen.getByText("Loading work content…")).toBeTruthy();

    rerender(
      <WorkContentReadOnlyList
        payloadStatus="UNAVAILABLE"
        reason="lineage missing"
      />,
    );
    expect(
      screen.getByText("Work content is unavailable. lineage missing"),
    ).toBeTruthy();

    rerender(
      <WorkContentReadOnlyList payloadStatus="ERROR" reason="projection failed" />,
    );
    expect(
      screen.getByText("Work content could not be loaded. projection failed"),
    ).toBeTruthy();

    rerender(<WorkContentReadOnlyList content={[]} />);
    expect(
      screen.getByText(
        "No work content items are available for this selection.",
      ),
    ).toBeTruthy();
  });

  it("exposes an accessible region name from the heading by default", () => {
    render(
      <WorkContentReadOnlyList
        content={[{ text: "Visible payload text", type: "TEXT" }]}
      />,
    );

    expect(screen.getByRole("region", { name: "Work contents" })).toBeTruthy();
    expect(screen.getByText("Visible payload text")).toBeTruthy();
  });
});
