import type {
  FactoryEmulatorInstanceState,
  FactoryEmulatorPlaybackSpeed,
} from "./factory-emulator-instance";

export interface FactoryEmulatorControlState {
  readonly disabledActions: readonly ("play" | "restart" | "step")[];
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
  return {
    disabledActions: [
      ...(executionUnavailable || state.mode === "history"
        ? (["play"] as const)
        : []),
      ...(restartUnavailable ? (["restart"] as const) : []),
      ...(executionUnavailable || state.mode === "history"
        ? (["step"] as const)
        : []),
    ],
    isPlaying: state.playback.status === "playing",
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
