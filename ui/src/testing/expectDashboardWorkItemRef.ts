import { expect } from "vitest";
import type { DashboardWorkItemRef } from "../api/dashboard";

/** Partial matcher for lineage-enriched dashboard work item refs in tests. */
export function expectDashboardWorkItemRef(
  partial: Partial<DashboardWorkItemRef>,
): DashboardWorkItemRef {
  return expect.objectContaining(partial) as DashboardWorkItemRef;
}
