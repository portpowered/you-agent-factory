import type { FactoryEvent } from "@you-agent-factory/client";
import { Heading, Text } from "@you-agent-factory/components";
import {
  FactoryEmulatorView,
  type FactoryEmulatorViewProps,
} from "@you-agent-factory/factory-visualizers";
import {
  Component,
  type ErrorInfo,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";
import { useStore } from "zustand";

import { useAppLocale } from "../../../i18n";
import type { CustomerFactoryEmulatorDemoFixture } from "../lib/customer-demo-fixtures";
import { customerFactoryEmulatorDemoFixtures } from "../lib/customer-demo-fixtures";
import {
  type CustomerFactoryEmulatorDemoWorld,
  customerFactoryEmulatorDemoReducer,
  selectCustomerFactoryEmulatorActivity,
} from "../lib/customer-demo-presentation";
import { getFactoryEmulatorMessages } from "../messages/factory-emulator";
import {
  createFactoryEmulatorInstance,
  type FactoryEmulatorInstance,
  type FactoryEmulatorInstanceState,
} from "../state/factory-emulator-instance";
import {
  selectFactoryEmulatorControls,
  selectFactoryEmulatorTimeline,
} from "../state/factory-emulator-presentation";
import { FactoryEmulatorSubmission } from "./factory-emulator-submission";

type DemoInstance = FactoryEmulatorInstance<
  FactoryEvent[],
  CustomerFactoryEmulatorDemoWorld
>;

export interface CustomerFactoryEmulatorDemosProps {
  readonly fixtures?: readonly CustomerFactoryEmulatorDemoFixture[];
  readonly locale?: string;
}

interface DemoSetupBoundaryProps {
  readonly children: ReactNode;
  readonly description: string;
  readonly errorMessage: string;
  readonly title: string;
}

interface DemoSetupBoundaryState {
  readonly failed: boolean;
}

class DemoSetupBoundary extends Component<
  DemoSetupBoundaryProps,
  DemoSetupBoundaryState
> {
  public state = { failed: false };

  public static getDerivedStateFromError(): DemoSetupBoundaryState {
    return { failed: true };
  }

  public componentDidCatch(_error: Error, _info: ErrorInfo): void {
    // The accessible contained state is the customer-facing diagnostic.
  }

  public render() {
    if (this.state.failed) {
      return (
        <article
          aria-label={this.props.title}
          className="min-w-0 rounded-xl border border-error bg-error-container p-4 text-on-error-container"
        >
          <Heading as="h2">{this.props.title}</Heading>
          <Text as="p">{this.props.description}</Text>
          <Text as="p" role="alert">
            {this.props.errorMessage}
          </Text>
        </article>
      );
    }
    return this.props.children;
  }
}

function createDemoInstance(
  fixture: CustomerFactoryEmulatorDemoFixture,
  locale: string,
): DemoInstance {
  return createFactoryEmulatorInstance<
    FactoryEvent[],
    CustomerFactoryEmulatorDemoWorld
  >({
    cloneState: structuredClone,
    factory: fixture.factory,
    locale,
    reducer: customerFactoryEmulatorDemoReducer,
    scenario: fixture.scenario,
  });
}

function runtimeStatus(
  state: FactoryEmulatorInstanceState<
    FactoryEvent[],
    CustomerFactoryEmulatorDemoWorld
  >,
  messages: ReturnType<typeof getFactoryEmulatorMessages>["demos"],
): FactoryEmulatorViewProps["controls"]["runtimeStatus"] {
  if (state.error || state.sessionStatus.phase === "error")
    return { label: messages.status.error, tone: "danger" };
  if (state.mode === "history")
    return { label: messages.status.history, tone: "warning" };
  if (state.replay.world.progress.counts.failed > 0)
    return { label: messages.status.failed, tone: "danger" };
  if (
    state.sessionStatus.phase === "closed" ||
    state.replay.world.progress.counts.completed > 0
  )
    return { label: messages.status.completed, tone: "success" };
  if (state.playback.status === "playing")
    return { label: messages.status.playing, tone: "success" };
  return { label: messages.status.ready, tone: "neutral" };
}

function localizedActivityLabel(
  label: string,
  messages: ReturnType<typeof getFactoryEmulatorMessages>["demos"],
): string {
  const englishLabels = getFactoryEmulatorMessages("en").demos.activityLabels;
  const key = Object.entries(englishLabels).find(
    ([, englishLabel]) => englishLabel === label,
  )?.[0] as keyof typeof englishLabels | undefined;
  return key ? messages.activityLabels[key] : label;
}

function DemoTerminalSummary({
  state,
  messages,
}: {
  readonly messages: ReturnType<typeof getFactoryEmulatorMessages>["demos"];
  readonly state: FactoryEmulatorInstanceState<
    FactoryEvent[],
    CustomerFactoryEmulatorDemoWorld
  >;
}) {
  const failed = state.replay.world.progress.counts.failed > 0;
  const completed = state.replay.world.progress.counts.completed > 0;
  if (!failed && !completed) return null;
  return (
    <section
      aria-label={failed ? messages.failureTitle : messages.successTitle}
      className={
        failed
          ? "rounded-xl border border-error bg-error-container p-3 text-on-error-container"
          : "rounded-xl border border-success bg-success-container p-3 text-on-success-container"
      }
    >
      <Heading as="h3">
        {failed ? messages.failureTitle : messages.successTitle}
      </Heading>
      <Text as="p">
        {failed ? messages.failureDescription : messages.successDescription}
      </Text>
    </section>
  );
}

function CustomerFactoryEmulatorDemo({
  fixture,
  locale,
}: {
  readonly fixture: CustomerFactoryEmulatorDemoFixture;
  readonly locale: string;
}) {
  const messages = getFactoryEmulatorMessages(locale).demos;
  const [instance] = useState(() => createDemoInstance(fixture, locale));
  const [setupError, setSetupError] = useState(false);
  const started = useRef(false);
  const state = useStore(instance.store);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void instance.commands.start().catch(() => setSetupError(true));
  }, [instance]);

  if (setupError) {
    return (
      <Text as="p" role="alert">
        {messages.error}
      </Text>
    );
  }

  const fixtureMessages = messages.fixtures[fixture.id];
  const controls = selectFactoryEmulatorControls(state);
  const timeline = selectFactoryEmulatorTimeline(state);
  const activity = selectCustomerFactoryEmulatorActivity(
    fixture,
    state.events,
    state.selectedTick,
    state.mode === "current",
  );
  const numberFormatter = new Intl.NumberFormat(locale);
  const duration = activity?.durationMs
    ? messages.duration(numberFormatter.format(activity.durationMs / 1_000))
    : undefined;
  const activityDetail = activity
    ? messages.activity(
        activity.workstation,
        activity.activityLabel
          ? localizedActivityLabel(activity.activityLabel, messages)
          : messages.activityFallback(activity.workstation),
        duration ?? messages.duration(numberFormatter.format(0)),
      )
    : messages.ready;

  return (
    <article
      aria-label={fixtureMessages.title}
      className="grid min-w-0 gap-4 rounded-xl border border-outline-variant bg-surface-container-lowest p-4"
      data-demo-id={fixture.id}
    >
      <header className="grid min-w-0 gap-2">
        <Heading as="h2">{fixtureMessages.title}</Heading>
        <Text as="p">{fixtureMessages.description}</Text>
        <Text as="p" aria-live="polite" role="status">
          {activityDetail}
        </Text>
      </header>
      {state.replay.world.progress.total === 0 &&
      state.sessionLifecycle === "started" ? (
        <Text as="p" role="status">
          {messages.empty}
        </Text>
      ) : null}
      <DemoTerminalSummary messages={messages} state={state} />
      <FactoryEmulatorView
        controls={{
          ...controls,
          formatTick: (tick) => numberFormatter.format(tick),
          onFollowLatest: instance.commands.followCurrent,
          onPause: instance.commands.pause,
          onPlay: instance.commands.play,
          onRestart: () => void instance.commands.restart(),
          onSelectTick: instance.commands.selectTick,
          onSpeedChange: instance.commands.setSpeed,
          onStep: () => void instance.commands.step(),
          runtimeStatus: runtimeStatus(state, messages),
          timeline: { messages: messages.timeline, state: timeline },
        }}
        submission={
          <FactoryEmulatorSubmission
            factory={fixture.factory}
            instance={instance}
            locale={locale}
          />
        }
        topology={{
          messages: messages.topology,
          state:
            state.replay.world.topology.nodes.length === 0
              ? { status: "empty" }
              : {
                  projection: {
                    activity: state.replay.world.activity,
                    load: state.replay.world.load,
                    topology: state.replay.world.topology,
                  },
                  status: "ready",
                },
        }}
        workProgress={{
          formatNumber: (value) => numberFormatter.format(value),
          messages: messages.progress,
          projection: state.replay.world.progress,
        }}
      />
    </article>
  );
}

export function CustomerFactoryEmulatorDemos({
  fixtures = Object.values(customerFactoryEmulatorDemoFixtures),
  locale: localeOverride,
}: CustomerFactoryEmulatorDemosProps) {
  const { locale } = useAppLocale(localeOverride);
  const messages = getFactoryEmulatorMessages(locale).demos;
  return (
    <section
      aria-label={messages.regionLabel}
      className="mx-auto grid w-full min-w-0 max-w-7xl gap-6 p-4 lg:grid-cols-2"
    >
      {fixtures.map((fixture) => {
        const fixtureMessages = messages.fixtures[fixture.id];
        return (
          <DemoSetupBoundary
            description={fixtureMessages.description}
            errorMessage={messages.error}
            key={fixture.id}
            title={fixtureMessages.title}
          >
            <CustomerFactoryEmulatorDemo fixture={fixture} locale={locale} />
          </DemoSetupBoundary>
        );
      })}
    </section>
  );
}
