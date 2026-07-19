import { SubmitWorkCard, type SubmitWorkCardProps } from "./submit-work-card";

/**
 * Transport-neutral rich Factory submission surface.
 *
 * Hosts own the draft, validation, mutation state, and all callbacks. This
 * component deliberately has no knowledge of sessions or transport clients.
 */
export type FactorySubmissionComposerProps = SubmitWorkCardProps;

export function FactorySubmissionComposer(
  props: FactorySubmissionComposerProps,
) {
  return <SubmitWorkCard {...props} />;
}
