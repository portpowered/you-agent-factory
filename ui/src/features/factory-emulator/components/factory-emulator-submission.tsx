import type { FactoryDefinition } from "@you-agent-factory/client";
import { useStore } from "zustand";

import { FactorySimpleSubmissionComposer } from "../../submit-work/components/composer/factory-simple-submission-composer";
import { getFactoryEmulatorMessages } from "../messages/factory-emulator";
import type { FactoryEmulatorInstance } from "../state/factory-emulator-instance";
import { selectFactoryEmulatorSubmission } from "../state/factory-emulator-submission";

export interface FactoryEmulatorSubmissionProps<State, World> {
  readonly factory: FactoryDefinition;
  readonly instance: FactoryEmulatorInstance<State, World>;
  readonly isRunTerminal?: boolean;
  readonly locale?: string;
}

/** Connect the shared controlled text composer to one local emulator instance. */
export function FactoryEmulatorSubmission<State, World>({
  factory,
  instance,
  isRunTerminal = false,
  locale,
}: FactoryEmulatorSubmissionProps<State, World>) {
  const messages = getFactoryEmulatorMessages(locale).submission;
  const state = useStore(instance.store, (current) =>
    selectFactoryEmulatorSubmission(current, factory, isRunTerminal),
  );

  return (
    <FactorySimpleSubmissionComposer
      {...state}
      locale={locale}
      onDraftChange={instance.commands.setDraft}
      onSubmit={async (submission) => {
        const outcome = await instance.commands.submit(submission);
        if (outcome.status === "disabled") throw new Error(outcome.reason);
      }}
      unavailableMessage={(reason) => messages.unavailable[reason]}
    />
  );
}
