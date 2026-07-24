// @vitest-environment happy-dom

import { axe } from "jest-axe";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";
import { PackageText } from "./package-text";

describe("PackageText", () => {
  it("renders children with accessible text content", async () => {
    renderPackageComponent(
      <main>
        <PackageText>Hello package</PackageText>
      </main>,
    );

    expect(screen.getByText("Hello package")).toBeInTheDocument();
    expect(await axe(document.body)).toHaveNoViolations();
  });

  it("supports user interactions on nested controls", async () => {
    const user = userEvent.setup();
    let clicked = false;

    renderPackageComponent(
      <PackageText>
        <button
          onClick={() => {
            clicked = true;
          }}
          type="button"
        >
          Activate
        </button>
      </PackageText>,
    );

    await user.click(screen.getByRole("button", { name: "Activate" }));
    expect(clicked).toBe(true);
  });

  it("applies the title variant class", () => {
    renderPackageComponent(
      <PackageText variant="title">Section heading</PackageText>,
    );

    expect(screen.getByText("Section heading")).toHaveClass("text-title-large");
  });
});
