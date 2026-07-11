import type { components } from "../generated/openapi";

type FactorySessionIdentity =
  | components["schemas"]["FactorySession"]
  | components["schemas"]["FactorySessionSummary"];

/** Removes legacy embedded dispatch data before a session enters identity caches. */
export function withoutEmbeddedSessionDispatches<
  T extends FactorySessionIdentity,
>(session: T): T {
  const runtime = session.runtime;
  if (!runtime || !("dispatches" in runtime)) {
    return session;
  }

  const runtimeWithoutDispatches = Object.fromEntries(
    Object.entries(runtime).filter(([key]) => key !== "dispatches"),
  ) as typeof runtime;

  return {
    ...session,
    runtime: runtimeWithoutDispatches,
  };
}
