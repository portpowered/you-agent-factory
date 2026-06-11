import type { components } from "../generated/openapi";

export type FactorySessionDurableSummary =
  components["schemas"]["FactorySessionDurableSummary"];
export type FactorySessionDurableReadModel =
  components["schemas"]["FactorySessionDurableReadModel"];
export type FactorySessionResult =
  components["schemas"]["FactorySessionResult"];
export type FactorySessionDispatchSummary =
  components["schemas"]["FactorySessionDispatchSummary"];
export type FactoryDispatch = components["schemas"]["FactoryDispatch"];
export type ListFactorySessionDispatchesResponse =
  components["schemas"]["ListFactorySessionDispatchesResponse"];
export type FactorySessionArtifactSummary =
  components["schemas"]["FactorySessionArtifactSummary"];
export type FactorySessionArtifactDetail =
  components["schemas"]["FactorySessionArtifactDetail"];
export type ListFactorySessionArtifactsResponse =
  components["schemas"]["ListFactorySessionArtifactsResponse"];
export type FactorySessionDurableLifecycleStatus =
  components["schemas"]["FactorySessionDurableLifecycleStatus"];
export type FactorySessionResultStatus =
  components["schemas"]["FactorySessionResultStatus"];
export type FactorySessionDurableActionAvailability =
  components["schemas"]["FactorySessionDurableActionAvailability"];

export type InspectionAdapterOutcome<T> =
  | { status: "loading" }
  | { status: "empty" }
  | { status: "error"; message: string; code?: string }
  | { status: "success"; data: T };

export type FixtureAdapterSimulation = "loading" | "empty" | "error";

export interface FixtureAdapterRequestOptions {
  errorCode?: string;
  errorMessage?: string;
  simulate?: FixtureAdapterSimulation;
}

export interface DurableSessionListInspectionData {
  scope: "persisted";
  sessions: FactorySessionDurableSummary[];
}
