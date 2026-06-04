import type { ReactNode } from "react";

import { ColorRoleAccentContrastShowcase } from "./color-role-accent-contrast";
import { ColorRoleNeutralSurfacesShowcase } from "./color-role-neutral-surfaces";
import { LayoutRoleShowcase } from "./layout-role-showcase";
import { TypographyRoleHierarchyShowcase } from "./typography-role-hierarchy";

function OverviewSection({
  children,
  heading,
  id,
}: {
  children: ReactNode;
  heading: string;
  id: string;
}) {
  return (
    <section
      aria-labelledby={id}
      className="flex flex-col gap-layout-section border-b border-outline pb-layout-section"
    >
      <h2 className="text-title-large text-on-surface" id={id}>
        {heading}
      </h2>
      {children}
    </section>
  );
}

/** Consolidated visual review surface for the full theme role migration (US-010). */
export function ThemeRoleMigrationOverview() {
  return (
    <article
      aria-label="Material theme role migration overview"
      className="flex flex-col gap-layout-page bg-background p-layout-inset-dialog text-on-surface"
    >
      <header className="flex flex-col gap-layout-tight">
        <h1 className="text-headline-medium text-on-surface">
          Theme role migration overview
        </h1>
        <p className="max-w-3xl text-body-medium text-on-surface-variant">
          Representative fixtures for color roles, typography, layout spacing,
          and palette switching. Use the header palette menu in dashboard
          stories to preview all five presets.
        </p>
      </header>

      <OverviewSection heading="Accent hierarchy" id="overview-accent">
        <ColorRoleAccentContrastShowcase />
      </OverviewSection>

      <OverviewSection heading="Neutral surfaces" id="overview-neutral">
        <ColorRoleNeutralSurfacesShowcase />
      </OverviewSection>

      <OverviewSection heading="Typography hierarchy" id="overview-typography">
        <TypographyRoleHierarchyShowcase />
      </OverviewSection>

      <OverviewSection heading="Layout primitives" id="overview-layout">
        <LayoutRoleShowcase />
      </OverviewSection>
    </article>
  );
}
