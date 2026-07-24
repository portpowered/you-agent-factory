import { useQuery } from "@tanstack/react-query";

import { getFactorySessionArtifact } from "../../../api/factory-sessions";
import {
  type FactorySessionArtifactDrilldown,
  type FactorySessionArtifactDrilldownLoadFailure,
  normalizeFactorySessionArtifactDrilldown,
  normalizeFactorySessionArtifactDrilldownLoadFailure,
} from "../lib/factory-session-artifact-drilldown";

export const FACTORY_SESSION_ARTIFACT_DRILLDOWN_QUERY_KEY = [
  "factory-session-artifact-drilldown",
] as const;

export type FactorySessionArtifactDrilldownViewState =
  | { status: "idle" }
  | { status: "loading" }
  | { artifact: FactorySessionArtifactDrilldown; status: "success" }
  | {
      failure: FactorySessionArtifactDrilldownLoadFailure;
      status: "error";
    };

export function useFactorySessionArtifactDrilldown(
  sessionID: string | null,
  artifactID: string | null,
  enabled: boolean,
): FactorySessionArtifactDrilldownViewState {
  const resolvedSessionID = sessionID?.trim() ?? "";
  const resolvedArtifactID = artifactID?.trim() ?? "";

  const query = useQuery({
    queryKey: [
      ...FACTORY_SESSION_ARTIFACT_DRILLDOWN_QUERY_KEY,
      resolvedSessionID,
      resolvedArtifactID,
    ],
    queryFn: async () =>
      normalizeFactorySessionArtifactDrilldown(
        await getFactorySessionArtifact(resolvedSessionID, resolvedArtifactID),
      ),
    enabled:
      enabled && resolvedSessionID.length > 0 && resolvedArtifactID.length > 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });

  if (
    !enabled ||
    resolvedSessionID.length === 0 ||
    resolvedArtifactID.length === 0
  ) {
    return { status: "idle" };
  }

  if (query.isPending || query.isFetching) {
    return { status: "loading" };
  }

  if (query.error) {
    return {
      failure: normalizeFactorySessionArtifactDrilldownLoadFailure(query.error),
      status: "error",
    };
  }

  if (!query.data) {
    return {
      failure: {
        kind: "unknown",
      },
      status: "error",
    };
  }

  return {
    artifact: query.data,
    status: "success",
  };
}
