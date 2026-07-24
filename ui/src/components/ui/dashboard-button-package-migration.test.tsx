import "@testing-library/jest-dom/vitest";

import { fireEvent, render, screen } from "@testing-library/react";
import {
  buttonVariants,
  IconButtonShell,
  Button as PackageButton,
  ButtonLink as PackageButtonLink,
} from "@you-agent-factory/components";
import { vi } from "vitest";

import { Button } from "./button";
import { ButtonLink } from "./button-link";
import { DashboardActionButton } from "./dashboard-action-button";
import { DashboardIconButtonShell } from "./dashboard-icon-button-shell";

describe("dashboard button package migration", () => {
  it("re-exports package button primitives from dashboard UI entrypoints", () => {
    expect(Button).toBe(PackageButton);
    expect(ButtonLink).toBe(PackageButtonLink);
    expect(DashboardIconButtonShell).toBe(IconButtonShell);
    expect(typeof buttonVariants).toBe("function");
  });

  it("renders representative migrated text, link, loading, and icon-only buttons", () => {
    render(
      <>
        <Button disabled type="button">
          Disabled
        </Button>
        <Button type="button">Save</Button>
        <ButtonLink href="/docs">Docs</ButtonLink>
        <DashboardActionButton executing type="button">
          Working
        </DashboardActionButton>
        <DashboardActionButton
          aria-label="Delete"
          executing
          iconOnly
          type="button"
        >
          x
        </DashboardActionButton>
        <DashboardIconButtonShell aria-label="Close" type="button">
          x
        </DashboardIconButtonShell>
      </>,
    );

    expect(screen.getByRole("button", { name: "Disabled" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    expect(screen.getByRole("link", { name: "Docs" })).toHaveAttribute(
      "href",
      "/docs",
    );

    const loadingButton = screen.getByRole("button", { name: "Working" });
    expect(loadingButton).toHaveAttribute("aria-busy", "true");
    expect(loadingButton).toBeDisabled();

    const loadingIconAction = screen.getByRole("button", { name: "Delete" });
    expect(loadingIconAction).toHaveAttribute("aria-busy", "true");
    expect(loadingIconAction).toBeDisabled();

    const iconButton = screen.getByRole("button", { name: "Close" });
    expect(iconButton.className).toContain("h-10");
    expect(iconButton.className).toContain("w-10");
  });

  it("blocks loading asChild projection and preserves semantic button tones", () => {
    const onBlockedClick = vi.fn();
    const onToneClick = vi.fn();

    render(
      <>
        <Button asChild loading tone="destructive">
          <a href="/blocked" onClick={onBlockedClick}>
            Blocked link
          </a>
        </Button>
        <Button onClick={onToneClick} tone="warning" type="button">
          Warning
        </Button>
        <ButtonLink href="/guide">Guide</ButtonLink>
      </>,
    );

    const blockedLink = screen.getByText("Blocked link");
    expect(blockedLink).toHaveAttribute("aria-busy", "true");
    expect(blockedLink).toHaveAttribute("aria-disabled", "true");
    expect(blockedLink).not.toHaveAttribute("href");
    fireEvent.click(blockedLink);
    fireEvent.keyDown(blockedLink, { key: "Enter" });
    expect(onBlockedClick).not.toHaveBeenCalled();

    const warningButton = screen.getByRole("button", { name: "Warning" });
    expect(warningButton.className).toContain("border-af-warning-border");
    fireEvent.click(warningButton);
    expect(onToneClick).toHaveBeenCalledTimes(1);

    expect(buttonVariants({ tone: "ghost" })).toContain("border-transparent");
    expect(screen.getByRole("link", { name: "Guide" })).toHaveAttribute(
      "href",
      "/guide",
    );
  });
});
