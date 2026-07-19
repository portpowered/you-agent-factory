// @vitest-environment happy-dom

import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryTimelineScrubber,
  type FactoryTimelineScrubberMessages,
  type FactoryTimelineScrubberProps,
} from "./factory-timeline-scrubber";

const messages: FactoryTimelineScrubberMessages = {
  alreadyFollowingLatest: "Following the latest tick.",
  currentMode: "Current Factory mode.",
  disabled: "Timeline selection is disabled by the host.",
  followLatest: "Follow latest",
  historyMode: "Viewing Factory history.",
  position: (selected, latest) => `Tick ${selected} of ${latest}`,
  regionLabel: "Factory replay timeline",
  sliderLabel: "Select replay tick",
  title: "Replay timeline",
  unavailable: "No replay ticks are available.",
};

const availableState: Extract<
  FactoryTimelineScrubberProps["state"],
  { status: "available" }
> = {
  earliestTick: 4,
  latestTick: 12,
  mode: "history",
  selectedTick: 7,
  status: "available",
};

function renderTimeline(props: Partial<FactoryTimelineScrubberProps> = {}) {
  const onFollowLatest = vi.fn();
  const onSelectTick = vi.fn();
  const view = render(
    <FactoryTimelineScrubber
      formatTick={String}
      messages={messages}
      onFollowLatest={onFollowLatest}
      onSelectTick={onSelectTick}
      state={availableState}
      {...props}
    />,
  );

  return { onFollowLatest, onSelectTick, ...view };
}

describe("FactoryTimelineScrubber controlled interaction", () => {
  it("emits pointer selection intent while keeping the visible tick controlled", () => {
    const { onSelectTick, rerender } = renderTimeline();
    const slider = screen.getByRole("slider", { name: "Select replay tick" });

    fireEvent.change(slider, { target: { value: "9" } });

    expect(onSelectTick).toHaveBeenCalledWith(9);
    expect(screen.getByText("Tick 7 of 12")).toBeInTheDocument();

    rerender(
      <FactoryTimelineScrubber
        formatTick={String}
        messages={messages}
        onFollowLatest={vi.fn()}
        onSelectTick={onSelectTick}
        state={{ ...availableState, selectedTick: 9 }}
      />,
    );
    expect(screen.getByText("Tick 9 of 12")).toBeInTheDocument();
  });

  it("supports native keyboard range selection without advancing time itself", () => {
    const { onSelectTick } = renderTimeline();
    const slider = screen.getByRole("slider", { name: "Select replay tick" });

    slider.focus();
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    fireEvent.change(slider, { target: { value: "8" } });

    expect(slider).toHaveFocus();
    expect(onSelectTick).toHaveBeenCalledWith(8);
    expect(screen.getByText("Tick 7 of 12")).toBeInTheDocument();
  });

  it("is keyboard ordered and has no automated accessibility violations", async () => {
    const user = userEvent.setup();
    const { container } = renderTimeline();

    await user.tab();
    expect(
      screen.getByRole("slider", { name: messages.sliderLabel }),
    ).toHaveFocus();
    await user.tab();
    expect(
      screen.getByRole("button", { name: messages.followLatest }),
    ).toHaveFocus();
    expect(await axe(container)).toHaveNoViolations();
  });

  it("reports follow-latest intent only from history mode", () => {
    const { onFollowLatest, rerender } = renderTimeline();
    const action = screen.getByRole("button", { name: "Follow latest" });

    fireEvent.click(action);
    expect(onFollowLatest).toHaveBeenCalledOnce();

    rerender(
      <FactoryTimelineScrubber
        formatTick={String}
        messages={messages}
        onFollowLatest={onFollowLatest}
        onSelectTick={vi.fn()}
        state={{ ...availableState, mode: "current", selectedTick: 12 }}
      />,
    );

    expect(action).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent(
      "Current Factory mode.",
    );
    expect(screen.getByText("Following the latest tick.")).toBeInTheDocument();
    fireEvent.click(action);
    expect(onFollowLatest).toHaveBeenCalledOnce();
  });
});

describe("FactoryTimelineScrubber state and localization", () => {
  it("makes unavailable, invalid, and host-disabled ranges explicit and inert", () => {
    const onSelectTick = vi.fn();
    const { rerender } = renderTimeline({
      onSelectTick,
      state: { status: "unavailable" },
    });

    expect(screen.getByRole("status")).toHaveTextContent(
      "No replay ticks are available.",
    );
    expect(screen.getByRole("slider")).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Follow latest" }),
    ).toBeDisabled();

    rerender(
      <FactoryTimelineScrubber
        formatTick={String}
        messages={messages}
        onFollowLatest={vi.fn()}
        onSelectTick={onSelectTick}
        state={{ ...availableState, earliestTick: 13 }}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "No replay ticks are available.",
    );

    rerender(
      <FactoryTimelineScrubber
        disabled
        formatTick={String}
        messages={messages}
        onFollowLatest={vi.fn()}
        onSelectTick={onSelectTick}
        state={availableState}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Timeline selection is disabled by the host.",
    );
    expect(screen.getByRole("slider")).toBeDisabled();
    expect(onSelectTick).not.toHaveBeenCalled();
  });

  it("uses host-provided tick formatting, labels, and mode messages", () => {
    const localizedMessages: FactoryTimelineScrubberMessages = {
      ...messages,
      historyMode: "Werkverlauf wird angezeigt.",
      position: (selected, latest) => `Schritt ${selected} von ${latest}`,
      sliderLabel: "Wiedergabeschritt auswählen",
    };

    renderTimeline({
      formatTick: new Intl.NumberFormat("de-DE").format,
      messages: localizedMessages,
      state: { ...availableState, latestTick: 12000, selectedTick: 7000 },
    });

    expect(screen.getByText("Schritt 7.000 von 12.000")).toBeInTheDocument();
    expect(
      screen.getByRole("slider", { name: "Wiedergabeschritt auswählen" }),
    ).toHaveAttribute("aria-valuetext", "7.000");
    expect(screen.getByRole("status")).toHaveTextContent(
      "Werkverlauf wird angezeigt.",
    );
  });
});
