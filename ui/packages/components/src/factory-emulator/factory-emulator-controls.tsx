import type { HTMLAttributes } from "react";

import { Button } from "../primitives/button";
import { cn } from "../utilities/cn";

export const FACTORY_EMULATOR_SPEEDS = [0.5, 1, 2, 4] as const;

export type FactoryEmulatorSpeed = (typeof FACTORY_EMULATOR_SPEEDS)[number];

export type FactoryEmulatorAction = "pause" | "play" | "restart" | "step";

export interface FactoryEmulatorRuntimeStatus {
  label: string;
  tone?: "danger" | "neutral" | "success" | "warning";
}

export interface FactoryEmulatorControlsProps
  extends Omit<HTMLAttributes<HTMLElement>, "onChange"> {
  disabledActions?: readonly FactoryEmulatorAction[];
  isPlaying: boolean;
  onPause: () => void;
  onPlay: () => void;
  onRestart: () => void;
  onSpeedChange: (speed: FactoryEmulatorSpeed) => void;
  onStep: () => void;
  runtimeStatus: FactoryEmulatorRuntimeStatus;
  showRuntimeStatus?: boolean;
  showSpeedControl?: boolean;
  speed: FactoryEmulatorSpeed;
}

const STATUS_TONE_CLASS: Record<
  NonNullable<FactoryEmulatorRuntimeStatus["tone"]>,
  string
> = {
  danger: "bg-error-container text-on-error-container",
  neutral: "bg-surface-container-high text-on-surface-variant",
  success: "bg-success-container text-on-success-container",
  warning: "bg-warning-container text-on-warning-container",
};

function isFactoryEmulatorSpeed(value: number): value is FactoryEmulatorSpeed {
  return FACTORY_EMULATOR_SPEEDS.includes(value as FactoryEmulatorSpeed);
}

export function FactoryEmulatorControls({
  className,
  disabledActions = [],
  isPlaying,
  onPause,
  onPlay,
  onRestart,
  onSpeedChange,
  onStep,
  runtimeStatus,
  showRuntimeStatus = true,
  showSpeedControl = true,
  speed,
  ...props
}: FactoryEmulatorControlsProps) {
  const disabled = new Set(disabledActions);

  return (
    <section
      aria-label="Factory emulator playback controls"
      className={cn(
        "flex w-full flex-wrap items-center gap-2 rounded-xl border border-outline bg-surface-container-low p-3",
        className,
      )}
      {...props}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Button disabled={disabled.has("play")} onClick={onPlay} size="sm">
          Play
        </Button>
        <Button disabled={disabled.has("pause")} onClick={onPause} size="sm" tone="secondary">
          Pause
        </Button>
        <Button disabled={disabled.has("step")} onClick={onStep} size="sm" tone="outline">
          Step
        </Button>
        <Button disabled={disabled.has("restart")} onClick={onRestart} size="sm" tone="ghost">
          Restart
        </Button>
      </div>

      {showSpeedControl ? <label className="ml-auto flex min-h-9 items-center gap-2 text-body-small font-medium text-on-surface-variant">
        <span>Speed</span>
        <select
          aria-label="Playback speed"
          className="min-h-9 rounded-lg border border-outline bg-surface-container-high px-2 text-body-small text-on-surface outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring"
          onChange={(event) => {
            const nextSpeed = Number(event.target.value);
            if (isFactoryEmulatorSpeed(nextSpeed)) onSpeedChange(nextSpeed);
          }}
          value={speed}
        >
          {FACTORY_EMULATOR_SPEEDS.map((option) => <option key={option} value={option}>{option}x</option>)}
        </select>
      </label> : null}

      {showRuntimeStatus ? <output
        aria-label="Runtime status"
        className={cn("min-h-9 rounded-full px-3 py-2 text-body-small font-medium", STATUS_TONE_CLASS[runtimeStatus.tone ?? "neutral"])}
        data-playing={isPlaying ? "true" : "false"}
      >
        {runtimeStatus.label}
      </output> : null}
    </section>
  );
}
