import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryEmulatorControls,
  type FactoryEmulatorControlsProps,
} from "./factory-emulator-controls";

const timelineMessages = {
  alreadyFollowingLatest: "Following the latest tick.",
  currentMode: "Showing the current Factory.",
  disabled: "Timeline selection is disabled by the host.",
  followLatest: "Follow latest",
  historyMode: "Viewing Factory history.",
  position: (selected: string, latest: string) =>
    `Tick ${selected} of ${latest}`,
  regionLabel: "Factory replay timeline",
  sliderLabel: "Select replay tick",
  title: "Replay timeline",
  unavailable: "No replay ticks are available.",
};

function renderControls(overrides: Partial<FactoryEmulatorControlsProps> = {}) {
  const props: FactoryEmulatorControlsProps = {
    formatTick: String,
    isPlaying: true,
    onFollowLatest: vi.fn(),
    onPause: vi.fn(),
    onPlay: vi.fn(),
    onRestart: vi.fn(),
    onSelectTick: vi.fn(),
    onSpeedChange: vi.fn(),
    onStep: vi.fn(),
    runtimeStatus: { label: "Running", tone: "success" },
    speed: 1,
    timeline: {
      messages: timelineMessages,
      state: {
        earliestTick: 0,
        latestTick: 8,
        mode: "history",
        selectedTick: 3,
        status: "available",
      },
    },
    ...overrides,
  };
  return { props, ...render(<FactoryEmulatorControls {...props} />) };
}

describe("FactoryEmulatorControls", () => {
  it("pauses before requesting a backward history selection", () => {
    const { props } = renderControls();
    fireEvent.change(
      screen.getByRole("slider", { name: "Select replay tick" }),
      { target: { value: "2" } },
    );
    expect(props.onPause).toHaveBeenCalledOnce();
    expect(props.onSelectTick).toHaveBeenCalledWith(2);
    expect(props.onPause.mock.invocationCallOrder[0]).toBeLessThan(
      props.onSelectTick.mock.invocationCallOrder[0],
    );
    expect(screen.getByText("Viewing Factory history.")).toBeDefined();
  });

  it.each([
    ["Play", "onPlay"],
    ["Step", "onStep"],
  ] as const)("returns to the latest tick before %s", (label, action) => {
    const { props } = renderControls();
    fireEvent.click(screen.getByRole("button", { name: label }));
    expect(props.onFollowLatest).toHaveBeenCalledOnce();
    expect(props[action]).toHaveBeenCalledOnce();
    expect(props.onFollowLatest.mock.invocationCallOrder[0]).toBeLessThan(
      props[action].mock.invocationCallOrder[0],
    );
  });

  it("does not follow latest before advancing while already current", () => {
    const { props } = renderControls({
      timeline: {
        messages: timelineMessages,
        state: {
          earliestTick: 0,
          latestTick: 8,
          mode: "current",
          selectedTick: 8,
          status: "available",
        },
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Step" }));
    expect(props.onFollowLatest).not.toHaveBeenCalled();
    expect(props.onStep).toHaveBeenCalledOnce();
  });

  it("delegates restart without reconstructing the host timeline", () => {
    const { props } = renderControls();
    fireEvent.click(screen.getByRole("button", { name: "Restart" }));
    expect(props.onRestart).toHaveBeenCalledOnce();
    expect(props.onPause).not.toHaveBeenCalled();
    expect(props.onFollowLatest).not.toHaveBeenCalled();
    expect(props.onSelectTick).not.toHaveBeenCalled();
  });
});
