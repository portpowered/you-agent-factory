import { Button, Heading, Text } from "@you-agent-factory/components/primitives";
import { type ChangeEvent, type HTMLAttributes, useId } from "react";

export type FactoryTimelineMode = "current" | "history";

export type FactoryTimelineScrubberState =
  | {
      earliestTick: number;
      latestTick: number;
      mode: FactoryTimelineMode;
      selectedTick: number;
      status: "available";
    }
  | {
      status: "unavailable";
    };

export interface FactoryTimelineScrubberMessages {
  alreadyFollowingLatest: string;
  disabled: string;
  followLatest: string;
  historyMode: string;
  currentMode: string;
  position: (
    formattedSelectedTick: string,
    formattedLatestTick: string,
  ) => string;
  regionLabel: string;
  sliderLabel: string;
  title: string;
  unavailable: string;
}

export interface FactoryTimelineScrubberProps
  extends Omit<HTMLAttributes<HTMLElement>, "children"> {
  disabled?: boolean;
  formatTick: (tick: number) => string;
  messages: FactoryTimelineScrubberMessages;
  onFollowLatest: () => void;
  onSelectTick: (tick: number) => void;
  state: FactoryTimelineScrubberState;
}

function isValidAvailableState(
  state: FactoryTimelineScrubberState,
): state is Extract<FactoryTimelineScrubberState, { status: "available" }> {
  return (
    state.status === "available" &&
    Number.isSafeInteger(state.earliestTick) &&
    Number.isSafeInteger(state.latestTick) &&
    Number.isSafeInteger(state.selectedTick) &&
    state.earliestTick >= 0 &&
    state.earliestTick <= state.latestTick &&
    state.selectedTick >= state.earliestTick &&
    state.selectedTick <= state.latestTick
  );
}

export function FactoryTimelineScrubber({
  className,
  disabled = false,
  formatTick,
  messages,
  onFollowLatest,
  onSelectTick,
  state,
  ...sectionProps
}: FactoryTimelineScrubberProps) {
  const descriptionId = useId();
  const classNames = ["factory-timeline-scrubber", className]
    .filter(Boolean)
    .join(" ");
  const availableState = isValidAvailableState(state) ? state : null;
  const isCurrent = availableState?.mode === "current";
  const interactionDisabled = disabled || availableState === null;
  const status = availableState
    ? disabled
      ? messages.disabled
      : availableState.mode === "history"
        ? messages.historyMode
        : messages.currentMode
    : messages.unavailable;
  const actionDescription = isCurrent
    ? messages.alreadyFollowingLatest
    : status;

  function handleSelection(event: ChangeEvent<HTMLInputElement>) {
    if (!availableState || disabled) {
      return;
    }

    const nextTick = event.currentTarget.valueAsNumber;
    if (
      Number.isSafeInteger(nextTick) &&
      nextTick >= availableState.earliestTick &&
      nextTick <= availableState.latestTick
    ) {
      onSelectTick(nextTick);
    }
  }

  return (
    <section
      aria-label={messages.regionLabel}
      className={classNames}
      data-timeline-mode={availableState?.mode ?? "unavailable"}
      {...sectionProps}
    >
      <header className="factory-timeline-scrubber__header">
        <Heading as="h2">{messages.title}</Heading>
        {availableState ? (
          <Text as="p" className="factory-timeline-scrubber__position">
            {messages.position(
              formatTick(availableState.selectedTick),
              formatTick(availableState.latestTick),
            )}
          </Text>
        ) : null}
      </header>

      <div className="factory-timeline-scrubber__controls">
        <input
          aria-describedby={descriptionId}
          aria-label={messages.sliderLabel}
          aria-valuetext={
            availableState ? formatTick(availableState.selectedTick) : undefined
          }
          className="factory-timeline-scrubber__range"
          disabled={interactionDisabled}
          max={availableState?.latestTick ?? 0}
          min={availableState?.earliestTick ?? 0}
          onChange={handleSelection}
          step={1}
          type="range"
          value={availableState?.selectedTick ?? 0}
        />
        <Button
          aria-describedby={descriptionId}
          disabled={interactionDisabled || isCurrent}
          onClick={onFollowLatest}
          tone="outline"
        >
          {messages.followLatest}
        </Button>
      </div>

      <Text
        as="p"
        className="factory-timeline-scrubber__status"
        id={!isCurrent || disabled ? descriptionId : undefined}
        role="status"
      >
        {status}
      </Text>
      {isCurrent && !disabled ? (
        <Text
          as="p"
          className="factory-timeline-scrubber__action-status"
          id={descriptionId}
        >
          {actionDescription}
        </Text>
      ) : null}
    </section>
  );
}
