interface VisitCountGuard {
  readonly type: string;
  readonly workstation?: string;
  readonly maxVisits?: number;
}

/** Evaluates the supported inclusive VISIT_COUNT guard set for one lineage. */
export function visitCountGuardsAllow(
  guards: readonly VisitCountGuard[],
  visits: Readonly<Record<string, number>>,
): boolean {
  return guards.every(
    ({ type, workstation, maxVisits }) =>
      type === "VISIT_COUNT" &&
      workstation !== undefined &&
      maxVisits !== undefined &&
      (visits[workstation] ?? 0) >= maxVisits,
  );
}

/** Carries the greatest contributing lineage counts and records this visit. */
export function visitsAfterTransition(
  inputs: readonly Readonly<Record<string, number>>[],
  workstation: string,
): Readonly<Record<string, number>> {
  const visits: Record<string, number> = {};
  for (const input of inputs) {
    for (const [name, count] of Object.entries(input)) {
      visits[name] = Math.max(visits[name] ?? 0, count);
    }
  }
  visits[workstation] = (visits[workstation] ?? 0) + 1;
  return visits;
}
