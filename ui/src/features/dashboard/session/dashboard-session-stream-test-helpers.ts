import type { MockEventSource } from "../../../testing/app-shell-test-utils";

export function requireEventStream(
  instances: MockEventSource[],
): MockEventSource {
  const stream = instances.at(-1);

  if (!stream) {
    throw new Error("expected factory event stream to be opened");
  }

  return stream;
}
