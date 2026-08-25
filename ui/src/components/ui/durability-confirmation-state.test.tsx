import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ConfirmationState } from "../../api/generated/openapi";
import {
  DurabilityConfirmationState,
  normalizeDurabilityConfirmationState,
} from "./durability-confirmation-state";

describe("DurabilityConfirmationState", () => {
  it("renders confirmed as an accessible success status", () => {
    render(
      <DurabilityConfirmationState
        label="Durability confirmation"
        state={ConfirmationState.CONFIRMED}
      />,
    );

    expect(
      screen.getByRole("status", {
        name: "Durability confirmation: CONFIRMED",
      }),
    ).toBeTruthy();
  });

  it("defaults absent and unknown values to unconfirmed", () => {
    expect(normalizeDurabilityConfirmationState(undefined)).toBe(
      ConfirmationState.UNCONFIRMED,
    );
    expect(normalizeDurabilityConfirmationState("LEGACY")).toBe(
      ConfirmationState.UNCONFIRMED,
    );

    render(<DurabilityConfirmationState label="Durability confirmation" />);

    expect(
      screen.getByRole("status", {
        name: "Durability confirmation: UNCONFIRMED",
      }),
    ).toBeTruthy();
  });
});
