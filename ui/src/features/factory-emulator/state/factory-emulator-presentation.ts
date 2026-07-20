import type {
  FactoryEmulatorInstanceState,
  FactoryEmulatorPlaybackSpeed,
} from "./factory-emulator-instance";

export interface FactoryEmulatorControlState {
  readonly disabledActions: readonly ("pause" | "play" | "restart" | "step")[];
  readonly isPlaying: boolean;
  readonly speed: FactoryEmulatorPlaybackSpeed;
}

export interface FactoryEmulatorTimelineState {
  readonly earliestTick: number;
  readonly latestTick: number;
  readonly mode: "current" | "history";
  readonly selectedTick: number;
  readonly status: "available";
}

export const selectFactoryEmulatorControls = <State, World>(
  state: FactoryEmulatorInstanceState<State, World>,
): FactoryEmulatorControlState => {
  const executionUnavailable =
    state.commandState === "running" ||
    state.sessionStatus.phase === "closed" ||
    state.sessionStatus.phase === "error";
  const restartUnavailable = state.commandState === "running";
  const isPlaying = state.playback.status === "playing";
  return {
    disabledActions: [
      ...(executionUnavailable || isPlaying ? (["play"] as const) : []),
      ...(!isPlaying ? (["pause"] as const) : []),
      ...(restartUnavailable ? (["restart"] as const) : []),
      ...(executionUnavailable ? (["step"] as const) : []),
    ],
    isPlaying,
    speed: state.playback.speed,
  };
};

export const selectFactoryEmulatorTimeline = <State, World>(
  state: FactoryEmulatorInstanceState<State, World>,
): FactoryEmulatorTimelineState => ({
  earliestTick: 0,
  latestTick: state.latestTick,
  mode: state.mode,
  selectedTick: state.selectedTick,
  status: "available",
});
