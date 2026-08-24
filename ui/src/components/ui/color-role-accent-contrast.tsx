const ACCENT_SWATCHES = [
  {
    containerClass: "bg-primary-container text-on-primary-container",
    id: "primary",
    label: "Primary",
    roleClass: "bg-primary text-on-primary",
    subtitle: "Brand yellow — most prominent accent",
  },
  {
    containerClass: "bg-secondary-container text-on-secondary-container",
    id: "secondary",
    label: "Secondary",
    roleClass: "bg-secondary text-on-secondary",
    subtitle: "Calm cyan supporting accent",
  },
  {
    containerClass: "bg-tertiary-container text-on-tertiary-container",
    id: "tertiary",
    label: "Tertiary",
    roleClass: "bg-tertiary text-on-tertiary",
    subtitle: "Calm violet supporting accent",
  },
] as const;

const LEGACY_REFERENCE_SWATCHES = [
  {
    id: "legacy-info",
    label: "Prior secondary hue (info foundation)",
    style: {
      backgroundColor: "var(--color-af-legacy-info)",
      color: "var(--color-af-legacy-info-ink)",
    },
    subtitle: "Semantic info / legacy vibrant cyan",
  },
  {
    id: "legacy-worker",
    label: "Prior tertiary hue (worker foundation)",
    style: {
      backgroundColor: "var(--color-af-legacy-worker)",
      color: "var(--color-af-legacy-worker-ink)",
    },
    subtitle: "Legacy vibrant violet chrome",
  },
] as const;

function AccentSwatch({
  containerClass,
  label,
  roleClass,
  subtitle,
}: (typeof ACCENT_SWATCHES)[number]) {
  return (
    <article className="grid gap-3 rounded-2xl border border-outline bg-surface-container-low p-4">
      <div>
        <h3 className="m-0 text-sm font-semibold text-on-surface">{label}</h3>
        <p className="m-0 pt-1 text-xs text-on-surface-variant">{subtitle}</p>
      </div>
      <div
        className={`flex h-14 items-center justify-center rounded-xl px-3 text-sm font-semibold ${roleClass}`}
      >
        Role fill
      </div>
      <div
        className={`rounded-xl px-3 py-3 text-sm font-medium ${containerClass}`}
      >
        Container + on-container ink
      </div>
    </article>
  );
}

function LegacyReferenceSwatch({
  label,
  style,
  subtitle,
}: (typeof LEGACY_REFERENCE_SWATCHES)[number]) {
  return (
    <article className="grid gap-3 rounded-2xl border border-dashed border-outline-variant bg-surface p-4">
      <div>
        <h3 className="m-0 text-sm font-semibold text-on-surface">{label}</h3>
        <p className="m-0 pt-1 text-xs text-on-surface-variant">{subtitle}</p>
      </div>
      <div
        className="flex h-14 items-center justify-center rounded-xl px-3 text-sm font-semibold"
        style={style}
      >
        Legacy reference
      </div>
    </article>
  );
}

export function ColorRoleAccentContrastShowcase() {
  return (
    <div className="grid max-w-5xl gap-6 rounded-2xl border border-outline bg-background p-6 text-on-surface">
      <header className="grid gap-2">
        <h2 className="m-0 font-display text-2xl tracking-[-0.03em]">
          Accent role contrast (US-003)
        </h2>
        <p className="m-0 max-w-3xl text-sm text-on-surface-variant">
          Primary stays the brightest accent. Secondary and tertiary use calmer
          foundation keys while semantic info keeps the prior vibrant cyan for
          status-only surfaces.
        </p>
      </header>

      <section
        aria-label="Material accent role swatches"
        className="grid gap-4 md:grid-cols-3"
      >
        {ACCENT_SWATCHES.map((swatch) => (
          <AccentSwatch key={swatch.id} {...swatch} />
        ))}
      </section>

      <section
        aria-label="Legacy vibrant accent references"
        className="grid gap-4 md:grid-cols-2"
      >
        {LEGACY_REFERENCE_SWATCHES.map((swatch) => (
          <LegacyReferenceSwatch key={swatch.id} {...swatch} />
        ))}
      </section>
    </div>
  );
}
