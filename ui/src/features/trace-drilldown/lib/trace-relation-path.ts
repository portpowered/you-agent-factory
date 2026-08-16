import type { TraceSelectionIdentity } from "./trace-selection";

export type TraceRelationPathKind = "predecessor" | "relation";

export interface TraceRelationPathEndpoint {
  dispatchID?: string;
  label: string;
  selectionIdentities: readonly TraceSelectionIdentity[];
  workID?: string;
}

/**
 * One relationship shared by the graph projection and the textual fallback.
 *
 * Graph node ids are intentionally absent here. They are disposable React
 * Flow projections; dispatch, Work, and attempt identities are the stable
 * values that the table and textual path can use to move focus.
 */
export interface TraceRelationPathEntry {
  id: string;
  kind: TraceRelationPathKind;
  relationType: string;
  requestID?: string;
  requiredState?: string;
  source: TraceRelationPathEndpoint;
  target: TraceRelationPathEndpoint;
}
