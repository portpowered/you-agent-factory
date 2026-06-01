import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { EditableConfigurationDiscardHeaderAction } from "./editable-configuration-discard-header-action";

describe("EditableConfigurationDiscardHeaderAction", () => {
  it("enables discard when allowed and not saving", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();

    render(
      <EditableConfigurationDiscardHeaderAction
        canDiscard
        isSaving={false}
        onClick={onClick}
      />,
    );

    const discardButton = screen.getByRole("button", {
      name: "Discard local changes",
    });
    expect(discardButton).toBeEnabled();

    await user.click(discardButton);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("disables discard when changes cannot be discarded", () => {
    render(
      <EditableConfigurationDiscardHeaderAction
        canDiscard={false}
        isSaving={false}
        onClick={() => undefined}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Discard local changes" }),
    ).toBeDisabled();
  });

  it("disables discard while save is in progress", () => {
    render(
      <EditableConfigurationDiscardHeaderAction
        canDiscard
        isSaving
        onClick={() => undefined}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Discard local changes" }),
    ).toBeDisabled();
  });

  it("uses localized discard copy when locale is provided", () => {
    render(
      <EditableConfigurationDiscardHeaderAction
        canDiscard
        isSaving={false}
        locale="ja"
        onClick={() => undefined}
      />,
    );

    expect(
      screen.getByRole("button", { name: "ローカル変更を破棄" }),
    ).toBeInTheDocument();
  });

  it("uses a custom aria label when provided", () => {
    render(
      <EditableConfigurationDiscardHeaderAction
        ariaLabel="Reset workstation draft"
        canDiscard
        isSaving={false}
        onClick={() => undefined}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Reset workstation draft" }),
    ).toBeInTheDocument();
  });
});
