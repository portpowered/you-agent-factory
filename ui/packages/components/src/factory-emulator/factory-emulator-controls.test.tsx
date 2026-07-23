import { axe } from "jest-axe";
import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing";
import {
  FactoryEmulatorControls,
  type FactoryEmulatorControlsProps,
} from "./factory-emulator-controls";

function renderControls(overrides: Partial<FactoryEmulatorControlsProps> = {}) {
  const props: FactoryEmulatorControlsProps = {
    isPlaying: false,
    onPause: vi.fn(),
    onPlay: vi.fn(),
    onRestart: vi.fn(),
    onSpeedChange: vi.fn(),
    onStep: vi.fn(),
    runtimeStatus: { label: "Ready", tone: "success" },
    speed: 1,
    ...overrides,
  };
  return {
    props,
    ...renderPackageComponent(<FactoryEmulatorControls {...props} />),
  };
}

describe("FactoryEmulatorControls", () => {
  it("renders controlled actions, supported speeds, and host runtime status", () => {
    renderControls({
      isPlaying: true,
      runtimeStatus: { label: "Running" },
      speed: 2,
    });
    for (const name of ["Play", "Pause", "Step", "Restart"])
      expect(screen.getByRole("button", { name })).toBeVisible();
    expect(
      screen.getByRole("combobox", { name: "Playback speed" }),
    ).toHaveValue("2");
    expect(
      screen.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["0.5x", "1x", "2x", "4x"]);
    expect(
      screen.getByRole("status", { name: "Runtime status" }),
    ).toHaveTextContent("Running");
    expect(screen.getByRole("status")).toHaveAttribute("data-playing", "true");
  });

  it("delegates each command and changes speed without applying a step multiplier", async () => {
    const user = userEvent.setup();
    const { props } = renderControls();
    await user.click(screen.getByRole("button", { name: "Play" }));
    await user.click(screen.getByRole("button", { name: "Pause" }));
    await user.click(screen.getByRole("button", { name: "Step" }));
    await user.click(screen.getByRole("button", { name: "Restart" }));
    await user.selectOptions(screen.getByRole("combobox"), "4");
    expect(props.onPlay).toHaveBeenCalledOnce();
    expect(props.onPause).toHaveBeenCalledOnce();
    expect(props.onStep).toHaveBeenCalledOnce();
    expect(props.onRestart).toHaveBeenCalledOnce();
    expect(props.onSpeedChange).toHaveBeenCalledExactlyOnceWith(4);
  });

  it("honors host-reported unavailable actions and has no accessibility violations", async () => {
    const { container } = renderControls({
      disabledActions: ["pause", "restart"],
    });
    expect(screen.getByRole("button", { name: "Pause" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Restart" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Play" })).toBeEnabled();
    expect(await axe(container)).toHaveNoViolations();
  });
});
