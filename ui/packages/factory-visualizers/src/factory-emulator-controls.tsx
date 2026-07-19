import {
  FactoryEmulatorControls as PlaybackControls,
  type FactoryEmulatorControlsProps as PlaybackControlsProps,
} from "@you-agent-factory/components";
import type { HTMLAttributes } from "react";
import {
  FactoryEmulatorErrorBoundary,
  type FactoryEmulatorFailure,
} from "./factory-emulator-error-boundary";
import {
  FactoryTimelineScrubber,
  type FactoryTimelineScrubberMessages,
  type FactoryTimelineScrubberState,
} from "./factory-timeline-scrubber";
import type { FactoryVisualizerError } from "./visualizer-error";

export interface FactoryEmulatorControlsProps
  extends Omit<HTMLAttributes<HTMLElement>, "children" | "onError">,
    Omit<
      PlaybackControlsProps,
      "onError" | "onPause" | "onPlay" | "onRestart" | "onStep"
    > {
  formatTick: (tick: number) => string;
  failure?: FactoryEmulatorFailure;
  onFollowLatest: () => void;
  onError?: (error: FactoryVisualizerError) => void;
  onPause: () => void;
  onPlay: () => void;
  onRestart: () => void;
  onSelectTick: (tick: number) => void;
  onStep: () => void;
  showPlaybackControls?: boolean;
  showTimelineScrubber?: boolean;
  timeline: FactoryEmulatorControlsTimeline;
}

export interface FactoryEmulatorControlsTimeline {
  disabled?: boolean;
  messages: FactoryTimelineScrubberMessages;
  state: FactoryTimelineScrubberState;
}

/** Controlled controls that make the distinction between current and historical replay explicit. */
export function FactoryEmulatorControls({
  className,
  failure,
  formatTick,
  onFollowLatest,
  onError,
  onPause,
  onPlay,
  onRestart,
  onSelectTick,
  onStep,
  showPlaybackControls = true,
  showTimelineScrubber = true,
  timeline,
  ...playbackProps
}: FactoryEmulatorControlsProps) {
  const isViewingHistory =
    timeline.state.status === "available" && timeline.state.mode === "history";

  function returnToLatestBefore(action: () => void) {
    if (isViewingHistory) onFollowLatest();
    action();
  }

  function selectTick(tick: number) {
    if (
      timeline.state.status === "available" &&
      tick < timeline.state.latestTick
    )
      onPause();
    onSelectTick(tick);
  }

  return (
    <FactoryEmulatorErrorBoundary
      failure={failure}
      onError={onError}
      regionLabel="Factory emulator controls"
    >
      <section
        aria-label="Factory emulator controls"
        className={["factory-emulator-controls", className]
          .filter(Boolean)
          .join(" ")}
      >
        {showPlaybackControls ? (
          <PlaybackControls
            {...playbackProps}
            onPause={onPause}
            onPlay={() => returnToLatestBefore(onPlay)}
            onRestart={onRestart}
            onStep={() => returnToLatestBefore(onStep)}
          />
        ) : null}
        {showTimelineScrubber ? (
          <FactoryTimelineScrubber
            disabled={timeline.disabled}
            formatTick={formatTick}
            messages={timeline.messages}
            onFollowLatest={onFollowLatest}
            onSelectTick={selectTick}
            state={timeline.state}
          />
        ) : null}
      </section>
    </FactoryEmulatorErrorBoundary>
  );
}
