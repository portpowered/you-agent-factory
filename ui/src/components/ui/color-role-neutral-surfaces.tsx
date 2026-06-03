function SurfaceLayerSample({
  description,
  label,
  surfaceClassName,
}: {
  description: string;
  label: string;
  surfaceClassName: string;
}) {
  return (
    <article
      className={`grid gap-2 rounded-xl border border-outline p-4 ${surfaceClassName}`}
    >
      <h3 className="m-0 text-sm font-semibold text-on-surface">{label}</h3>
      <p className="m-0 text-xs text-on-surface-variant">{description}</p>
    </article>
  );
}

export function ColorRoleNeutralSurfacesShowcase() {
  return (
    <section
      aria-label="Material neutral surface role layers"
      className="grid max-w-4xl gap-6 rounded-2xl border border-outline bg-background p-6 text-on-surface"
    >
      <header className="grid gap-2">
        <h2 className="m-0 font-display text-2xl tracking-tight">
          Neutral surface layering (US-005)
        </h2>
        <p className="m-0 max-w-2xl text-sm text-on-surface-variant">
          Page background through highest containers use role tokens so panels,
          cards, and shells share one elevation ladder.
        </p>
      </header>
      <div className="grid gap-3 rounded-2xl border border-outline-variant bg-surface p-4">
        <p className="m-0 text-xs font-semibold uppercase tracking-wide text-on-surface-variant">
          Nested on default surface
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <SurfaceLayerSample
            description="Low-emphasis panels, table shells, chart backdrops."
            label="surface-container-low"
            surfaceClassName="bg-surface-container-low"
          />
          <SurfaceLayerSample
            description="Standard elevated panels between low and high."
            label="surface-container"
            surfaceClassName="bg-surface-container"
          />
          <SurfaceLayerSample
            description="Raised cards, inputs, dialogs, popovers."
            label="surface-container-high"
            surfaceClassName="bg-surface-container-high"
          />
          <SurfaceLayerSample
            description="Highest emphasis containers within a surface."
            label="surface-container-highest"
            surfaceClassName="bg-surface-container-highest"
          />
        </div>
      </div>
    </section>
  );
}
