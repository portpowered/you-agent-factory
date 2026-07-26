import { DashboardActionButton } from "../../../../components/ui/dashboard-action-button";

export function FactoryGraphEdgeWaypointControls({
  addWaypointLabel,
  edgeKindLabel,
  edgeSourceLabel,
  edgeTargetLabel,
  fieldKindLabel,
  fieldSourceLabel,
  fieldTargetLabel,
  onAddWaypoint,
  onRemoveWaypoint,
  removeWaypointLabel,
  selectedEdgeLabel,
  waypointCount,
}: {
  addWaypointLabel: string;
  edgeKindLabel: string;
  edgeSourceLabel: string;
  edgeTargetLabel: string;
  fieldKindLabel: string;
  fieldSourceLabel: string;
  fieldTargetLabel: string;
  onAddWaypoint: () => void;
  onRemoveWaypoint: (waypointIndex: number) => void;
  removeWaypointLabel: (index: number) => string;
  selectedEdgeLabel: string;
  waypointCount: number;
}) {
  return (
    <section
      aria-label={selectedEdgeLabel}
      className="absolute bottom-4 left-4 z-20 max-w-sm rounded-xl border border-outline bg-surface-container-high p-3 shadow-sm"
      data-factory-edge-waypoint-controls
    >
      <p className="m-0 text-xs font-semibold text-on-surface-subtle">
        {selectedEdgeLabel}
      </p>
      <dl className="mt-2 grid gap-1 text-sm text-on-surface">
        <div className="grid grid-cols-[5rem_1fr] gap-2">
          <dt className="text-on-surface-subtle">{fieldSourceLabel}</dt>
          <dd className="m-0 truncate font-mono text-xs">{edgeSourceLabel}</dd>
        </div>
        <div className="grid grid-cols-[5rem_1fr] gap-2">
          <dt className="text-on-surface-subtle">{fieldTargetLabel}</dt>
          <dd className="m-0 truncate font-mono text-xs">{edgeTargetLabel}</dd>
        </div>
        <div className="grid grid-cols-[5rem_1fr] gap-2">
          <dt className="text-on-surface-subtle">{fieldKindLabel}</dt>
          <dd className="m-0">{edgeKindLabel}</dd>
        </div>
      </dl>
      <div className="mt-3 flex flex-wrap gap-2">
        <DashboardActionButton onClick={onAddWaypoint} tone="secondary">
          {addWaypointLabel}
        </DashboardActionButton>
        {Array.from({ length: waypointCount }, (_, index) => (
          <DashboardActionButton
            // biome-ignore lint/suspicious/noArrayIndexKey: waypoint order is the stable identity for remove controls.
            key={`remove-waypoint-${index}`}
            onClick={() => onRemoveWaypoint(index)}
            tone="secondary"
          >
            {removeWaypointLabel(index)}
          </DashboardActionButton>
        ))}
      </div>
    </section>
  );
}
