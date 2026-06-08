import { CurrentSelectionSelectableButton } from "../../features/current-selection/base/components/presentation/current-selection-selectable-button";
import {
  DashboardHeaderOptionMenuItem,
  DashboardHeaderOptionMenuSurface,
} from "../../features/header/components/dashboard-header-option-menu";
import { DashboardStatusPill } from "./dashboard-status-pill";

export function PrimaryEmphasisSurfacesShowcase() {
  return (
    <div className="grid max-w-xl gap-6 rounded-2xl border border-outline bg-background p-6 text-on-surface">
      <header className="grid gap-2">
        <h2 className="m-0 font-display text-2xl tracking-[-0.03em]">
          Primary emphasis surfaces
        </h2>
        <p className="m-0 text-sm text-on-surface-variant">
          Shared dashboard primitives that should keep accent-ink foreground on
          primary-container emphasis after palette switches.
        </p>
      </header>

      <section
        aria-label="Primary emphasis runtime surfaces"
        className="grid gap-4"
      >
        <DashboardStatusPill data-testid="status-pill-active" tone="active">
          Active status
        </DashboardStatusPill>

        <CurrentSelectionSelectableButton
          data-testid="current-selection-selected"
          selected
        >
          Selected work item
        </CurrentSelectionSelectableButton>

        <DashboardHeaderOptionMenuSurface aria-label="Palette menu" role="menu">
          <DashboardHeaderOptionMenuItem isSelected onClick={() => undefined}>
            Factory Light
          </DashboardHeaderOptionMenuItem>
        </DashboardHeaderOptionMenuSurface>
      </section>
    </div>
  );
}
