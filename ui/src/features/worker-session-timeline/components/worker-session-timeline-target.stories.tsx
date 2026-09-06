import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";

import type { WorkerSessionObservation } from "../../../api/worker-sessions";
import type { UseWorkerSessionTimelineResult } from "../hooks/useWorkerSessionTimeline";
import { getWorkerSessionTimelineMessages } from "../messages/worker-session-timeline";
import { WorkerSessionTimelineWidget } from "./worker-session-timeline-widget";

const meta = {
  title: "Worker Session Timeline/Target origins",
  component: WorkerSessionTimelineWidget,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof WorkerSessionTimelineWidget>;

export default meta;
type Story = StoryObj<typeof meta>;

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

const observations: WorkerSessionObservation[] = [
  observation("worker-session-story-1", "attempt-story-1", {
    durationBasis: "RECORDED_TIMESTAMPS",
    durationMillis: 1842,
    model: "gpt-5",
    providerSession: {
      id: "provider-story-1",
      kind: "session_id",
      provider: "codex",
    },
    providerSessionAvailable: true,
    reasoningEffort: "medium",
    recordingHealth: "COMPLETE",
    state: "COMPLETED",
    workId: "work-terminal-story",
    workName: "Terminal story",
  }),
  observation("worker-session-story-2", "attempt-story-2", {
    durationBasis: "UNAVAILABLE",
    failure: {
      agentRunFailureClass: null,
      detail: "Provider stopped responding.",
      kind: "PROVIDER_FAILURE",
      providerContinuationFailureKind: null,
      providerContinuationOutcome: null,
      providerFailureKind: "TIMEOUT",
    },
    providerSession: null,
    recordingHealth: "INCOMPLETE",
    recordingHealthReason: "CORRUPT_TAIL",
    state: "FAILED",
  }),
];

export const MultipleObservations: Story = {
  args: {
    enabled: false,
    factorySessionID: "factory-session-story",
    loadWorkerSessionTargets: async () => observations,
    stateOverride: readyEmptyTimelineState(),
    workID: "work-terminal-story",
    workerSessionID: null,
  },
  decorators: [
    (Story) => (
      <QueryClientProvider client={queryClient}>
        <Story />
      </QueryClientProvider>
    ),
  ],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const messages = getWorkerSessionTimelineMessages("en");
    const secondTarget = await canvas.findByRole("button", {
      name: messages.openWorkerSessionTargetLabel(
        "worker-session-story-2",
        "work-terminal-story",
      ),
    });
    const secondTargetCard = secondTarget.closest("li");
    await expect(secondTargetCard).not.toBeNull();
    await expect(
      await within(secondTargetCard as HTMLElement).findByText(
        "Durability confirmation: UNCONFIRMED",
      ),
    ).toBeVisible();
    await expect(
      await within(secondTargetCard as HTMLElement).findByText(
        "Recording: INCOMPLETE",
      ),
    ).toBeVisible();
    await expect(
      await within(secondTargetCard as HTMLElement).findByText(
        "Provider stopped responding.",
      ),
    ).toBeVisible();

    await userEvent.click(secondTarget);
    await expect(secondTarget).toHaveAttribute("aria-pressed", "true");
    await expect(
      canvasElement.querySelector(
        '[data-worker-session-timeline-worker-session-id="worker-session-story-2"]',
      ),
    ).not.toBeNull();
  },
};

function observation(
  workerSessionId: string,
  attemptId: string,
  overrides: Partial<WorkerSessionObservation> = {},
): WorkerSessionObservation {
  return {
    attemptId,
    direct: false,
    durationBasis: "UNAVAILABLE",
    durationMillis: null,
    endedAt: null,
    parse: { errors: [], ignored: 0 },
    providerSessionAvailable: false,
    startedAt: null,
    state: "RUNNING",
    transcript: "AVAILABLE",
    turnId: null,
    workIds: ["work-terminal-story"],
    workerSessionId,
    ...overrides,
  } as WorkerSessionObservation;
}

function readyEmptyTimelineState(): UseWorkerSessionTimelineResult {
  return {
    acknowledgedCursor: null,
    entries: [],
    isFollowing: false,
    records: [],
    recordingHealth: null,
    recordingHealthReason: null,
    reconnectCursor: null,
    replaySummary: null,
    retry: () => undefined,
    sourceError: null,
    status: "ready-empty",
    streamGenerationId: null,
    terminalDelivery: null,
  };
}
