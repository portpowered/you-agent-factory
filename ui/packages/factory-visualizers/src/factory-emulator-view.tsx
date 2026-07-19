import type { HTMLAttributes, ReactNode } from "react";

import {
  FactoryEmulatorControls,
  type FactoryEmulatorControlsProps,
} from "./factory-emulator-controls";
import {
  FactoryEmulatorErrorBoundary,
  type FactoryEmulatorFailure,
} from "./factory-emulator-error-boundary";
import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayProps,
} from "./factory-topology-replay";
import type { FactoryVisualizerError } from "./visualizer-error";
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
  extends Omit<HTMLAttributes<HTMLElement>, "children" | "onError"> {
  controls: FactoryEmulatorControlsProps;
  failure?: FactoryEmulatorFailure;
  onError?: (error: FactoryVisualizerError) => void;
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
  failure,
  onError,
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
    <FactoryEmulatorErrorBoundary
      failure={failure}
      onError={onError}
      regionLabel="Factory emulator view"
    >
      <section
        aria-label="Factory emulator view"
        className={["factory-emulator-view", className]
          .filter(Boolean)
          .join(" ")}
        data-preset={preset}
        {...sectionProps}
      >
        {showControls ? (
          <FactoryEmulatorControls
            {...controls}
            onError={combineErrorReports(controls.onError, onError)}
            showPlaybackControls={regions.playbackControls}
            showRuntimeStatus={regions.runtimeStatus}
            showSpeedControl={regions.speedControl}
            showTimelineScrubber={regions.timelineScrubber}
          />
        ) : regions.timelineScrubber ? (
          <FactoryEmulatorControls
            {...controls}
            onError={combineErrorReports(controls.onError, onError)}
            showPlaybackControls={false}
            showRuntimeStatus={false}
            showSpeedControl={false}
            showTimelineScrubber
          />
        ) : null}
        <FactoryTopologyReplay
          {...topology}
          onError={combineErrorReports(topology.onError, onError)}
        />
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
    </FactoryEmulatorErrorBoundary>
  );
}

function combineErrorReports(
  primary: ((error: FactoryVisualizerError) => void) | undefined,
  secondary: ((error: FactoryVisualizerError) => void) | undefined,
) {
  if (!secondary || primary === secondary) return primary;
  if (!primary) return secondary;
  return (error: FactoryVisualizerError) => {
    primary(error);
    secondary(error);
  };
}
