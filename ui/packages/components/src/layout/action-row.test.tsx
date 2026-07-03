// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { ActionRow } from "./action-row";

describe("ActionRow", () => {
  it("renders status and action sections from host-provided content", () => {
    renderPackageComponent(
      <ActionRow
        actions={<button type="button">Save</button>}
        statuses={<span>Ready</span>}
      />,
    );

    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    expect(
      screen.getByText("Ready").closest('[data-action-row-section="statuses"]'),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Save" })
        .closest('[data-action-row-section="actions"]'),
    ).toBeTruthy();
  });

  it("returns null when no host content is provided", () => {
    const { container } = renderPackageComponent(<ActionRow />);
    expect(container).toBeEmptyDOMElement();
  });

  it("applies flex-wrap layout classes for responsive action-row wrapping", () => {
    const { container } = renderPackageComponent(
      <ActionRow
        actions={
          <>
            <button type="button">Primary action</button>
            <button type="button">Secondary action</button>
            <button type="button">Tertiary action</button>
          </>
        }
        statuses={
          <span>
            Long host-supplied status label that should wrap without clipping
            controls
          </span>
        }
      />,
    );

    const row = container.firstElementChild;
    expect(row?.className).toContain("flex-wrap");
    expect(row?.className).toContain("max-md:justify-start");

    const sections = container.querySelectorAll("[data-action-row-section]");
    expect(sections).toHaveLength(2);
    for (const section of sections) {
      expect(section.className).toContain("min-w-0");
      expect(section.className).toContain("flex-wrap");
    }
  });

  it("keeps statuses before grouped actions with enough controls to wrap", () => {
    const { container } = renderPackageComponent(
      <div style={{ width: "200px" }}>
        <ActionRow
          actions={
            <>
              <button type="button">Discard</button>
              <button type="button">Save draft</button>
              <button type="button">Publish</button>
              <button type="button">Archive</button>
            </>
          }
          statuses={<span>Draft changes pending review</span>}
        />
      </div>,
    );

    const sections = container.querySelectorAll("[data-action-row-section]");
    expect(sections).toHaveLength(2);
    expect(sections[0]?.getAttribute("data-action-row-section")).toBe(
      "statuses",
    );
    expect(sections[1]?.getAttribute("data-action-row-section")).toBe(
      "actions",
    );
    expect(screen.getAllByRole("button")).toHaveLength(4);
  });
});
