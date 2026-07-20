import type { FactoryDefinition } from "@you-agent-factory/client";
import type { FactoryEmulatorSession } from "@you-agent-factory/factory-emulator";
import type { StoreApi } from "zustand/vanilla";

import {
  type FactorySimpleSubmissionEligibilityInput,
  type FactorySimpleTextSubmission,
  resolveFactorySimpleSubmissionAvailability,
} from "../../submit-work/public";
import { getFactoryEmulatorMessages } from "../messages/factory-emulator";
import type {
  FactoryEmulatorCommandOutcome,
  FactoryEmulatorInstanceState,
} from "./factory-emulator-instance";

export interface FactoryEmulatorSubmissionStoreState {
  readonly draft: string;
  readonly nextOrdinal: number;
  readonly status: "idle" | "submitting";
}

export interface FactoryEmulatorSubmissionState
  extends FactorySimpleSubmissionEligibilityInput {
  readonly draft: string;
  readonly isSubmitting: boolean;
  readonly submissionError?: string;
}

export interface FactoryEmulatorSubmissionCommands {
  setDraft(draft: string): void;
  submit(
    submission: FactorySimpleTextSubmission,
  ): Promise<FactoryEmulatorCommandOutcome>;
}

type RunEmulatorCommand = (
  command: "submit",
  invoke: () => Promise<unknown>,
) => Promise<void>;

function rejectionMessage(rejection: unknown): string {
  return rejection instanceof Error ? rejection.message : String(rejection);
}

function disabled(reason: string): FactoryEmulatorCommandOutcome {
  return { command: "submit", reason, status: "disabled" };
}

export function createFactoryEmulatorSubmissionCommands<State, World>(
  factory: FactoryDefinition,
  session: FactoryEmulatorSession,
  store: StoreApi<FactoryEmulatorInstanceState<State, World>>,
  run: RunEmulatorCommand,
  locale?: string,
): FactoryEmulatorSubmissionCommands {
  const messages = getFactoryEmulatorMessages(locale).submission;
  return {
    setDraft: (draft) => {
      const { submission } = store.getState();
      store.setState({ submission: { ...submission, draft } });
    },
    submit: async (submission) => {
      const state = store.getState();
      const availability = resolveFactorySimpleSubmissionAvailability(
        selectFactoryEmulatorSubmission(state, factory),
      );
      if (availability.kind === "unavailable") {
        return disabled(messages.unavailable[availability.reason]);
      }
      const [content] = submission.content;
      if (
        submission.content.length !== 1 ||
        content?.type !== "text" ||
        content.text.trim().length === 0
      ) {
        return disabled(messages.blank);
      }
      if (submission.workTypeName !== availability.workTypeName) {
        return disabled(messages.wrongWorkType);
      }
      const workType = factory.workTypes?.find(
        ({ name }) => name === availability.workTypeName,
      );
      const initialState = workType?.states.find(
        ({ type }) => type === "INITIAL",
      );
      if (workType === undefined || initialState === undefined) {
        return disabled(messages.unavailable["no-default"]);
      }

      store.setState({
        submission: { ...state.submission, status: "submitting" },
      });
      try {
        const invoke = async () => {
          await session.submit({
            input: content.text,
            name: `website-interactive-work-${state.submission.nextOrdinal}`,
            state: initialState.name,
            workType: workType.name,
          });
          store.setState({
            error: undefined,
            submission: {
              draft: "",
              nextOrdinal: state.submission.nextOrdinal + 1,
              status: "idle",
            },
          });
        };
        await run("submit", invoke);
        return { status: "accepted" };
      } catch (rejection) {
        store.setState({
          error: {
            command: "submit",
            kind: "submission-rejected",
            message: rejectionMessage(rejection),
            recoverable: true,
          },
          submission: { ...store.getState().submission, status: "idle" },
        });
        throw rejection;
      }
    },
  };
}

export const selectFactoryEmulatorSubmission = <State, World>(
  state: FactoryEmulatorInstanceState<State, World>,
  factory: FactoryDefinition,
): FactoryEmulatorSubmissionState => ({
  draft: state.submission.draft,
  factoryState:
    state.sessionLifecycle === "closed" ||
    state.sessionStatus.phase === "closed"
      ? "closed"
      : state.sessionStatus.phase === "error"
        ? "error"
        : state.sessionLifecycle === "pre-start"
          ? "loading"
          : "active",
  isCurrent: state.mode === "current",
  isSubmitting: state.submission.status === "submitting",
  ...(state.error?.kind === "submission-rejected"
    ? { submissionError: state.error.message }
    : {}),
  workTypes: (factory.workTypes ?? []).map((workType) => ({
    handlingBehavior: workType.handlingBehavior,
    isSubmitEligible:
      workType.states.filter(({ type }) => type === "INITIAL").length === 1,
    name: workType.name,
  })),
});
