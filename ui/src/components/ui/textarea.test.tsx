import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { vi } from "vitest";

import { Textarea, textareaVariants } from "./textarea";

describe("Textarea", () => {
  it("applies the shared field surface with scroll constraints", () => {
    render(<Textarea aria-label="Factory notes" />);

    const textarea = screen.getByLabelText("Factory notes");
    expect(textarea.className).toContain("min-h-28");
    expect(textarea.className).toContain("max-h-52");
    expect(textarea.className).toContain("overflow-y-auto");
    expect(textarea.className).toContain("resize-none");
    expect(textarea.className).toContain("border-outline");
  });

  it("exposes variant class generation for sibling primitive composition", () => {
    expect(textareaVariants({ className: "custom-textarea" })).toContain(
      "custom-textarea",
    );
    expect(textareaVariants()).toContain("overflow-y-auto");
  });

  it("keeps the plain variant free of field chrome and resize handles", () => {
    render(<Textarea aria-label="Inline notes" variant="plain" />);

    const textarea = screen.getByLabelText("Inline notes");
    expect(textarea.className).toContain("resize-none");
    expect(textarea.className).not.toContain("border-outline");
    expect(textarea.className).not.toContain("overflow-y-auto");
  });

  it("allows long content to scroll inside the field surface", () => {
    const longText = "line\n".repeat(40);
    render(<Textarea aria-label="Factory notes" defaultValue={longText} />);

    const textarea =
      screen.getByLabelText<HTMLTextAreaElement>("Factory notes");
    Object.defineProperty(textarea, "scrollHeight", {
      configurable: true,
      value: 480,
    });
    Object.defineProperty(textarea, "clientHeight", {
      configurable: true,
      value: 208,
    });

    expect(textarea.scrollHeight).toBeGreaterThan(textarea.clientHeight);
    textarea.scrollTop = 64;
    expect(textarea.scrollTop).toBe(64);
  });

  it("preserves authoring behavior and disabled treatment", () => {
    const onChange = vi.fn();

    const { rerender } = render(
      <Textarea aria-label="Factory notes" onChange={onChange} />,
    );

    const textarea =
      screen.getByLabelText<HTMLTextAreaElement>("Factory notes");
    fireEvent.change(textarea, { target: { value: "Updated notes" } });
    expect(onChange).toHaveBeenCalled();

    fireEvent.paste(textarea, {
      clipboardData: {
        getData: () => " pasted",
      },
    });

    textarea.focus();
    expect(textarea).toHaveFocus();

    rerender(
      <Textarea aria-label="Factory notes" disabled onChange={onChange} />,
    );
    expect(screen.getByLabelText("Factory notes")).toBeDisabled();
  });
});
