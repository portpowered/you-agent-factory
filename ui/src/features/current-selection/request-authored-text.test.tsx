import { render, screen } from "@testing-library/react";
import { AuthoredBodyText, RequestAuthoredText } from "./detail-card-shared";

describe("AuthoredBodyText", () => {
  it("renders headings, lists, inline code, and fenced code blocks for markdown-authored request text", () => {
    const { container } = render(
      <AuthoredBodyText
        value={[
          "## Review checklist",
          "",
          "- Confirm the latest diff",
          "- Run `bun test` before approval",
          "",
          "```text",
          "bun test",
          "```",
        ].join("\n")}
      />,
    );

    expect(screen.getByRole("heading", { level: 2, name: "Review checklist" })).toBeTruthy();
    expect(screen.getByRole("list")).toBeTruthy();
    expect(screen.getByText("Confirm the latest diff")).toBeTruthy();
    const codeBlocks = screen.getAllByText("bun test", { selector: "code" });
    expect(codeBlocks).toHaveLength(2);
    expect(screen.getAllByText("bun test", { selector: "pre code" })).toHaveLength(1);
    expect(container.firstElementChild?.className).toContain("[&_code]:rounded-sm");
    expect(container.firstElementChild?.className).toContain("[&_code]:px-1");
    expect(container.firstElementChild?.className).toContain("[&_code]:py-0.5");
  });

  it("renders plain text as readable fallback without requiring markdown syntax", () => {
    const { container } = render(
      <AuthoredBodyText
        value={[
          "Review the current story before approval.",
          "Keep the existing response rendering unchanged.",
        ].join("\n")}
      />,
    );

    expect(screen.queryByRole("heading")).toBeNull();
    expect(screen.queryByRole("list")).toBeNull();
    expect(container.querySelectorAll("p")).toHaveLength(1);
    expect(container.textContent).toContain("Review the current story before approval.");
    expect(container.textContent).toContain("Keep the existing response rendering unchanged.");
  });

  it("renders embedded raw html as inert text", () => {
    const { container } = render(
      <AuthoredBodyText value={'<button>danger</button>\n\n<script>alert("xss")</script>'} />,
    );

    expect(screen.queryByRole("button", { name: "danger" })).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(container.textContent).toContain("<button>danger</button>");
    expect(container.textContent).toContain('<script>alert("xss")</script>');
  });

  it("keeps the request-authored wrapper aligned with the shared formatter", () => {
    render(<RequestAuthoredText value="## Review checklist" />);

    expect(screen.getByRole("heading", { level: 2, name: "Review checklist" })).toBeTruthy();
  });
});
