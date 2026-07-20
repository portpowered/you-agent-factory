import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@you-agent-factory/components";
import { useEffect, useState } from "react";
import { expect, within } from "storybook/test";

import { FactoryEmulatorView } from "./factory-emulator-view";
import { createFactoryTopologyProjection } from "./testing/factory-topology-projection";

const meta = {
  title: "Factory Visualizers/FactoryEmulatorView",
  component: FactoryEmulatorView,
  args: {
    controls: {
      formatTick: String,
      isPlaying: false,
      onFollowLatest: () => undefined,
      onPause: () => undefined,
      onPlay: () => undefined,
      onRestart: () => undefined,
      onSelectTick: () => undefined,
      onSpeedChange: () => undefined,
      onStep: () => undefined,
      runtimeStatus: { label: "Ready", tone: "success" },
      speed: 1,
      timeline: {
        messages: {
          alreadyFollowingLatest: "Following latest.",
          currentMode: "Current Factory.",
          disabled: "Timeline disabled.",
          followLatest: "Follow latest",
          historyMode: "Viewing history.",
          position: (selected: string, latest: string) =>
            `Tick ${selected} of ${latest}`,
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
    },
    submission: <button type="button">Submit Work</button>,
    topology: {
      messages: {
        activeDispatches: (count: number) => `${count} active Dispatches`,
        annotationsHidden: "Show annotations",
        annotationsVisible: "Hide annotations",
        empty: "No topology.",
        failed: "Topology failed.",
        inactiveDispatches: "No active Dispatches",
        imageFailed: "Annotation image unavailable.",
        imageLoading: "Loading annotation image.",
        legendActiveRoute: "Active route",
        legendInactiveRoute: "Inactive route",
        legendLabel: "Topology legend",
        loading: "Loading topology.",
        nodeLabel: (kind: string, label: string) => `${kind}: ${label}`,
        regionLabel: "Factory topology",
        resourceOccupancy: (occupied: number, capacity: number) =>
          `${occupied}/${capacity}`,
        resourceOccupancyUnavailable: "Occupancy unavailable",
        retry: "Retry",
        selectedNode: "Selected",
        viewportControlsLabel: "Topology viewport controls",
        workStateCount: (count: number) => `${count} Work`,
        workStateCountUnavailable: "Work unavailable",
      },
      state: { projection: createFactoryTopologyProjection(), status: "ready" },
    },
    workProgress: {
      formatNumber: String,
      messages: {
        categories: {
          active: { plural: String, singular: String },
          completed: { plural: String, singular: String },
          failed: { plural: String, singular: String },
          queued: { plural: String, singular: String },
          unclassified: { plural: String, singular: String },
        },
        empty: "No Work.",
        regionLabel: "Work progress",
        title: "Work progress",
        total: (count: string) => `${count} Work total`,
      },
      projection: {
        active: [],
        completed: [],
        failed: [],
        queued: [],
        unclassified: [],
        counts: {
          active: 0,
          completed: 0,
          failed: 0,
          queued: 0,
          unclassified: 0,
        },
        selectedTick: 2,
        total: 0,
      },
    },
  },
  parameters: { layout: "padded" },
} satisfies Meta<typeof FactoryEmulatorView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Full: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Play" })).toBeVisible();
    await expect(
      canvas.getByRole("region", { name: "Factory topology" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Submit Work" }),
    ).toBeVisible();
  },
};
export const LoadingInitial: Story = {
  args: {
    controls: {
      ...meta.args.controls,
      disabledActions: ["pause", "play", "restart", "step"],
      runtimeStatus: { label: "Starting", tone: "neutral" },
      timeline: {
        ...meta.args.controls.timeline,
        state: { status: "unavailable" },
      },
    },
    submission: (
      <button disabled type="button">
        Submit Work
      </button>
    ),
    topology: {
      ...meta.args.topology,
      state: { status: "loading" },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("status", { name: "Runtime status" }),
    ).toHaveTextContent("Starting");
    await expect(canvas.getByText("Loading topology.")).toBeVisible();
  },
};
export const Empty: Story = {
  args: {
    topology: { ...meta.args.topology, state: { status: "empty" } },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No topology.")).toBeVisible();
    await expect(
      canvas.getByRole("region", { name: "Work progress" }),
    ).toHaveAttribute("data-work-progress-total", "0");
  },
};
export const Terminal: Story = {
  args: {
    controls: {
      ...meta.args.controls,
      disabledActions: ["pause", "play", "step"],
      runtimeStatus: { label: "Completed", tone: "success" },
    },
    submission: (
      <button disabled type="button">
        Submit Work
      </button>
    ),
    workProgress: {
      ...meta.args.workProgress,
      projection: {
        active: [],
        completed: [{ id: "work-1" }],
        failed: [],
        queued: [],
        unclassified: [],
        counts: {
          active: 0,
          completed: 1,
          failed: 0,
          queued: 0,
          unclassified: 0,
        },
        selectedTick: 2,
        total: 1,
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("status", { name: "Runtime status" }),
    ).toHaveTextContent("Completed");
    await expect(
      canvas.getByRole("region", { name: "Work progress" }),
    ).toHaveAttribute("data-work-progress-total", "1");
  },
};
export const Compact: Story = { args: { preset: "compact" } };
export const DisplayOnly: Story = { args: { preset: "display-only" } };
export const AccessiblePlayback: Story = {
  render: () => <AccessiblePlaybackHost />,
};
export const HostFailure: Story = {
  args: {
    failure: {
      message: "The host could not prepare this emulator view.",
      recoveryAction: { label: "Try again", onRecover: () => undefined },
    },
  },
};

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

function AccessiblePlaybackHost() {
  const prefersReducedMotion = usePrefersReducedMotion();
  const [latestTick, setLatestTick] = useState(2);
  const [selectedTick, setSelectedTick] = useState(2);
  const [mode, setMode] = useState<"current" | "history">("current");
  const [isPlaying, setIsPlaying] = useState(() => !prefersReducedMotion);
  const [submissions, setSubmissions] = useState(0);

  useEffect(() => {
    if (prefersReducedMotion) setIsPlaying(false);
  }, [prefersReducedMotion]);

  useEffect(() => {
    if (!isPlaying) return;
    const timer = window.setInterval(() => {
      setLatestTick((current) => {
        const next = current + 1;
        setSelectedTick(next);
        return next;
      });
    }, 400);
    return () => window.clearInterval(timer);
  }, [isPlaying]);

  function followLatest() {
    setMode("current");
    setSelectedTick(latestTick);
  }

  function restart() {
    setIsPlaying(false);
    setLatestTick(2);
    setSelectedTick(2);
    setMode("current");
    setSubmissions(0);
  }

  return (
    <FactoryEmulatorView
      controls={{
        ...meta.args.controls,
        disabledActions: isPlaying ? ["play"] : ["pause"],
        isPlaying,
        onFollowLatest: followLatest,
        onPause: () => setIsPlaying(false),
        onPlay: () => {
          followLatest();
          setIsPlaying(true);
        },
        onRestart: restart,
        onSelectTick: (tick) => {
          setIsPlaying(false);
          setMode("history");
          setSelectedTick(tick);
        },
        onStep: () => {
          const next = latestTick + 1;
          setLatestTick(next);
          setSelectedTick(next);
          setMode("current");
        },
        runtimeStatus: {
          label: isPlaying
            ? "Playing"
            : mode === "history"
              ? "Viewing history"
              : "Paused",
          tone: isPlaying
            ? "success"
            : mode === "history"
              ? "warning"
              : "neutral",
        },
        timeline: {
          ...meta.args.controls.timeline,
          state: {
            earliestTick: 0,
            latestTick,
            mode,
            selectedTick,
            status: "available",
          },
        },
      }}
      submission={
        <div>
          <Button onClick={() => setSubmissions((count) => count + 1)}>
            Submit Work
          </Button>
          <output aria-live="polite">Submissions: {submissions}</output>
        </div>
      }
      topology={meta.args.topology}
      workProgress={meta.args.workProgress}
    />
  );
}

function usePrefersReducedMotion() {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(
    () => window.matchMedia?.(REDUCED_MOTION_QUERY).matches ?? false,
  );

  useEffect(() => {
    const mediaQuery = window.matchMedia?.(REDUCED_MOTION_QUERY);
    if (!mediaQuery) return;
    const updatePreference = () => setPrefersReducedMotion(mediaQuery.matches);
    updatePreference();
    mediaQuery.addEventListener("change", updatePreference);
    return () => mediaQuery.removeEventListener("change", updatePreference);
  }, []);

  return prefersReducedMotion;
}
