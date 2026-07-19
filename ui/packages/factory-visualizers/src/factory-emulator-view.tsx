import type { HTMLAttributes, ReactNode } from "react";

import {
  FactoryEmulatorControls,
  type FactoryEmulatorControlsProps,
} from "./factory-emulator-controls";
import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayProps,
} from "./factory-topology-replay";
import {
  WorkProgressVisualizer,
  type WorkProgressVisualizerProps,
} from "./work-progress-visualizer";

export type FactoryEmulatorViewPreset = "compact" | "display-only" | "full";

export interface FactoryEmulatorViewVisibility {
  playbackControls?: boolean;
  runtimeStatus?: boolean;
  speedControl?: boolean;
  submission?: boolean;
  timelineScrubber?: boolean;
  workProgress?: boolean;
}

export interface FactoryEmulatorViewProps
  extends Omit<HTMLAttributes<HTMLElement>, "children"> {
  controls: FactoryEmulatorControlsProps;
  preset?: FactoryEmulatorViewPreset;
  submission?: ReactNode;
  topology: FactoryTopologyReplayProps;
  visibility?: FactoryEmulatorViewVisibility;
  workProgress: WorkProgressVisualizerProps;
}

const PRESET_VISIBILITY: Record<
  FactoryEmulatorViewPreset,
  Required<FactoryEmulatorViewVisibility>
> = {
  full: {
    playbackControls: true,
    runtimeStatus: true,
    speedControl: true,
    submission: true,
    timelineScrubber: true,
    workProgress: true,
  },
  compact: {
    playbackControls: true,
    runtimeStatus: true,
    speedControl: false,
    submission: false,
    timelineScrubber: true,
    workProgress: true,
  },
  "display-only": {
    playbackControls: false,
    runtimeStatus: false,
    speedControl: false,
    submission: false,
    timelineScrubber: false,
    workProgress: false,
  },
};

/** A controlled Factory emulator layout. Presets select regions; hosts retain all state and behavior. */
export function FactoryEmulatorView({
  className,
  controls,
  preset = "full",
  submission,
  topology,
  visibility,
  workProgress,
  ...sectionProps
}: FactoryEmulatorViewProps) {
  const regions = { ...PRESET_VISIBILITY[preset], ...visibility };
  const showControls =
    regions.playbackControls || regions.speedControl || regions.runtimeStatus;

  return (
    <section
      aria-label="Factory emulator view"
      className={["factory-emulator-view", className].filter(Boolean).join(" ")}
      data-preset={preset}
      {...sectionProps}
    >
      {showControls ? (
        <FactoryEmulatorControls
          {...controls}
          showPlaybackControls={regions.playbackControls}
          showRuntimeStatus={regions.runtimeStatus}
          showSpeedControl={regions.speedControl}
          showTimelineScrubber={regions.timelineScrubber}
        />
      ) : regions.timelineScrubber ? (
        <FactoryEmulatorControls
          {...controls}
          showPlaybackControls={false}
          showRuntimeStatus={false}
          showSpeedControl={false}
          showTimelineScrubber
        />
      ) : null}
      <FactoryTopologyReplay {...topology} />
      {regions.workProgress ? (
        <WorkProgressVisualizer {...workProgress} />
      ) : null}
      {regions.submission && submission ? (
        <section
          aria-label="Factory emulator submission"
          className="factory-emulator-view__submission"
        >
          {submission}
        </section>
      ) : null}
    </section>
  );
}
