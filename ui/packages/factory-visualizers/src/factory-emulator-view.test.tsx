import { render, screen, waitFor } from "@testing-library/react";
import { createFactoryGraphSource } from "@you-agent-factory/factory-graph";
import type {
  FactoryWorkProgressCategory,
  FactoryWorkProgressProjection,
} from "@you-agent-factory/factory-replay";
import { describe, expect, it, vi } from "vitest";

import type { FactoryEmulatorControlsProps } from "./factory-emulator-controls";
import { FactoryEmulatorView } from "./factory-emulator-view";
import type { FactoryTopologyReplayProps } from "./factory-topology-replay";
import { createFactoryTopologyProjection } from "./testing/factory-topology-projection";
import type { WorkProgressVisualizerProps } from "./work-progress-visualizer";

const controls: FactoryEmulatorControlsProps = {
  formatTick: String,
  isPlaying: false,
  onFollowLatest: vi.fn(),
  onPause: vi.fn(),
  onPlay: vi.fn(),
  onRestart: vi.fn(),
  onSelectTick: vi.fn(),
  onSpeedChange: vi.fn(),
  onStep: vi.fn(),
  runtimeStatus: { label: "Ready" },
  speed: 1,
  timeline: {
    messages: {
      alreadyFollowingLatest: "Following latest.",
      currentMode: "Current Factory.",
      disabled: "Timeline disabled.",
      followLatest: "Follow latest",
      historyMode: "Viewing history.",
      position: (selected, latest) => `${selected}/${latest}`,
      regionLabel: "Factory timeline",
      sliderLabel: "Select tick",
      title: "Timeline",
      unavailable: "No timeline.",
    },
    state: {
      earliestTick: 0,
      latestTick: 2,
      mode: "current",
      selectedTick: 2,
      status: "available",
    },
  },
};

const topology: FactoryTopologyReplayProps = {
  messages: {
    activeDispatches: (count) => `${count} active Dispatches`,
    annotationsHidden: "Show annotations",
    annotationsVisible: "Hide annotations",
    empty: "No topology.",
    failed: "Topology failed.",
    inactiveDispatches: "No active Dispatches",
    imageFailed: "Annotation image unavailable.",
    imageLoading: "Loading annotation image.",
    loading: "Loading topology.",
    nodeLabel: (kind, label) => `${kind}: ${label}`,
    regionLabel: "Factory topology",
    resourceOccupancy: (occupied, capacity) => `${occupied}/${capacity}`,
    resourceOccupancyUnavailable: "Occupancy unavailable",
    retry: "Retry",
    selectedNode: "Selected",
    workStateCount: (count) => `${count} Work`,
    workStateCountUnavailable: "Work unavailable",
  },
  state: {
    source: createFactoryGraphSource({
      factory: { name: "Topology fixture" },
      runtime: createFactoryTopologyProjection(),
      selectedTick: 2,
    }),
    status: "ready",
  },
};

const categories: Record<
  FactoryWorkProgressCategory,
  { plural: (count: string) => string; singular: (count: string) => string }
> = {
  active: {
    plural: (count) => `${count} active`,
    singular: (count) => `${count} active`,
  },
  completed: {
    plural: (count) => `${count} completed`,
    singular: (count) => `${count} completed`,
  },
  failed: {
    plural: (count) => `${count} failed`,
    singular: (count) => `${count} failed`,
  },
  queued: {
    plural: (count) => `${count} queued`,
    singular: (count) => `${count} queued`,
  },
  unclassified: {
    plural: (count) => `${count} unclassified`,
    singular: (count) => `${count} unclassified`,
  },
};
const projection: FactoryWorkProgressProjection = {
  active: [],
  completed: [],
  failed: [],
  queued: [],
  unclassified: [],
  counts: { active: 0, completed: 0, failed: 0, queued: 0, unclassified: 0 },
  selectedTick: 2,
  total: 0,
};
const workProgress: WorkProgressVisualizerProps = {
  formatNumber: String,
  messages: {
    categories,
    empty: "No Work.",
    regionLabel: "Work progress",
    title: "Work progress",
    total: (count) => `${count} Work total`,
  },
  projection,
};
const submission = <button type="button">Submit Work</button>;

function BrokenSubmission(): never {
  throw new Error("must-not-leak");
}

describe("FactoryEmulatorView", () => {
  it("renders the documented full vertical composition", () => {
    render(
      <FactoryEmulatorView
        controls={controls}
        submission={submission}
        topology={topology}
        workProgress={workProgress}
      />,
    );
    expect(screen.getByRole("button", { name: "Play" })).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Factory timeline" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Factory topology" }),
    ).toBeTruthy();
    expect(screen.getByRole("region", { name: "Work progress" })).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Factory emulator submission" }),
    ).toContain(screen.getByRole("button", { name: "Submit Work" }));
  });

  it("uses compact and display-only presets without leaving hidden regions in the DOM", () => {
    const { rerender } = render(
      <FactoryEmulatorView
        controls={controls}
        preset="compact"
        submission={submission}
        topology={topology}
        workProgress={workProgress}
      />,
    );
    expect(
      screen.queryByRole("combobox", { name: "Playback speed" }),
    ).toBeNull();
    expect(
      screen.queryByRole("region", { name: "Factory emulator submission" }),
    ).toBeNull();
    rerender(
      <FactoryEmulatorView
        controls={controls}
        preset="display-only"
        topology={topology}
        workProgress={workProgress}
      />,
    );
    expect(screen.queryByRole("button", { name: "Play" })).toBeNull();
    expect(
      screen.queryByRole("region", { name: "Factory timeline" }),
    ).toBeNull();
    expect(screen.queryByRole("region", { name: "Work progress" })).toBeNull();
    expect(
      screen.getByRole("region", { name: "Factory topology" }),
    ).toBeTruthy();
  });

  it("lets every visibility override take precedence over its preset", () => {
    render(
      <FactoryEmulatorView
        controls={controls}
        preset="display-only"
        submission={submission}
        topology={topology}
        visibility={{
          playbackControls: true,
          runtimeStatus: true,
          speedControl: true,
          submission: true,
          timelineScrubber: true,
          workProgress: true,
        }}
        workProgress={workProgress}
      />,
    );
    expect(screen.getByRole("button", { name: "Play" })).toBeTruthy();
    expect(
      screen.getByRole("combobox", { name: "Playback speed" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Factory timeline" }),
    ).toBeTruthy();
    expect(screen.getByRole("region", { name: "Work progress" })).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Factory emulator submission" }),
    ).toBeTruthy();
  });
});

describe("FactoryEmulatorView visibility overrides", () => {
  it.each([
    ["speedControl", "combobox", "Playback speed"],
    ["runtimeStatus", "status", "Runtime status"],
  ] as const)(
    "renders the %s override without playback actions",
    (region, role, name) => {
      render(
        <FactoryEmulatorView
          controls={controls}
          preset="display-only"
          topology={topology}
          visibility={{ [region]: true }}
          workProgress={workProgress}
        />,
      );
      expect(screen.queryByRole("button", { name: "Play" })).toBeNull();
      expect(screen.getByRole(role, { name })).toBeTruthy();
    },
  );

  it("omits speed and status when their overrides are disabled", () => {
    render(
      <FactoryEmulatorView
        controls={controls}
        preset="full"
        topology={topology}
        visibility={{
          playbackControls: false,
          runtimeStatus: false,
          speedControl: false,
        }}
        workProgress={workProgress}
      />,
    );
    expect(screen.queryByRole("button", { name: "Play" })).toBeNull();
    expect(
      screen.queryByRole("combobox", { name: "Playback speed" }),
    ).toBeNull();
    expect(screen.queryByRole("status", { name: "Runtime status" })).toBeNull();
  });
});

describe("FactoryEmulatorView failure containment", () => {
  it("contains composition failures and forwards a safe diagnostic", async () => {
    const onError = vi.fn();
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    render(
      <div>
        <button type="button">Host action</button>
        <FactoryEmulatorView
          controls={controls}
          onError={onError}
          submission={<BrokenSubmission />}
          topology={topology}
          workProgress={workProgress}
        />
      </div>,
    );
    expect(screen.getByRole("button", { name: "Host action" })).toBeTruthy();
    expect(
      screen
        .getByRole("region", { name: "Factory emulator view" })
        .contains(screen.getByRole("alert")),
    ).toBe(true);
    await waitFor(() =>
      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "render", recoverable: true }),
      ),
    );
    expect(JSON.stringify(onError.mock.calls)).not.toContain("must-not-leak");
    consoleError.mockRestore();
  });

  it("renders the host-supplied recovery action as a local failure state", () => {
    const onRecover = vi.fn();
    render(
      <FactoryEmulatorView
        controls={controls}
        failure={{
          message: "The host could not assemble the emulator.",
          recoveryAction: { label: "Reconnect", onRecover },
        }}
        topology={topology}
        workProgress={workProgress}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "The host could not assemble the emulator.",
    );
    screen.getByRole("button", { name: "Reconnect" }).click();
    expect(onRecover).toHaveBeenCalledOnce();
    expect(
      screen.queryByRole("region", { name: "Factory topology" }),
    ).toBeNull();
  });
});
