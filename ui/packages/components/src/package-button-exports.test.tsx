// @vitest-environment happy-dom

import {
  Button,
  ButtonLink,
  buttonVariants,
  IconButtonShell,
} from "@you-agent-factory/components";
import {
  Button as PrimitiveButton,
  ButtonLink as PrimitiveButtonLink,
  IconButtonShell as PrimitiveIconButtonShell,
  buttonVariants as primitiveButtonVariants,
} from "@you-agent-factory/components/primitives";
import { describe, expect, it } from "vitest";
import { renderPackageComponent, screen } from "./testing/render";

describe("@you-agent-factory/components button exports", () => {
  it("imports button primitives from the package root and renders accessible controls", () => {
    renderPackageComponent(
      <>
        <Button>Save changes</Button>
        <ButtonLink href="/docs">Open docs</ButtonLink>
        <IconButtonShell aria-label="Remove item">
          <span aria-hidden="true">x</span>
        </IconButtonShell>
      </>,
    );

    expect(
      screen.getByRole("button", { name: "Save changes" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open docs" })).toHaveAttribute(
      "href",
      "/docs",
    );
    expect(
      screen.getByRole("button", { name: "Remove item" }),
    ).toBeInTheDocument();
    expect(typeof buttonVariants).toBe("function");
  });

  it("imports button primitives from the primitives deep export path", () => {
    renderPackageComponent(
      <>
        <PrimitiveButton>Confirm</PrimitiveButton>
        <PrimitiveButtonLink href="/settings">Settings</PrimitiveButtonLink>
        <PrimitiveIconButtonShell aria-label="Close panel">
          <span aria-hidden="true">x</span>
        </PrimitiveIconButtonShell>
      </>,
    );

    expect(screen.getByRole("button", { name: "Confirm" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings" })).toHaveAttribute(
      "href",
      "/settings",
    );
    expect(
      screen.getByRole("button", { name: "Close panel" }),
    ).toBeInTheDocument();
    expect(typeof primitiveButtonVariants).toBe("function");
  });
});
