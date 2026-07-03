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
});
