import { Button } from "./button";
import { ExpandablePanelTrigger } from "./expandable-panel-trigger";
import {
  StandardListSelection,
  StandardListSelectionItem,
} from "./standard-list-selection";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./table";

export const OVERLAY_HOVER_VERIFICATION_PALETTES = [
  "factory-dark",
  "factory-light",
] as const;

export function ColorRoleOverlayHoverSurfacesShowcase() {
  return (
    <section
      aria-label="Shared primitive overlay hover role verification"
      className="grid max-w-3xl gap-6 rounded-2xl border border-outline bg-background p-6 text-on-surface"
    >
      <header className="grid gap-2">
        <h2 className="m-0 font-display text-2xl tracking-tight">
          Overlay hover migration (surface-container roles)
        </h2>
        <p className="m-0 max-w-2xl text-sm text-on-surface-variant">
          Ghost, outline, and secondary buttons; table row hover and selection;
          neutral list rows; and expandable panel triggers use surface-container
          elevation steps instead of translucent overlay tokens.
        </p>
      </header>

      <div className="flex flex-wrap gap-2">
        <Button data-testid="hover-ghost-button" tone="ghost">
          Ghost hover
        </Button>
        <Button data-testid="hover-outline-button" tone="outline">
          Outline hover
        </Button>
        <Button data-testid="hover-secondary-button" tone="secondary">
          Secondary hover
        </Button>
      </div>

      <Table aria-label="Overlay hover table verification">
        <TableHeader>
          <TableRow>
            <TableHead>Column</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow data-testid="hover-table-row">
            <TableCell>Hover row</TableCell>
          </TableRow>
          <TableRow data-state="selected" data-testid="selected-table-row">
            <TableCell>Selected row</TableCell>
          </TableRow>
        </TableBody>
      </Table>

      <StandardListSelection aria-label="Overlay hover list verification">
        <StandardListSelectionItem data-testid="hover-list-row">
          Neutral list row
        </StandardListSelectionItem>
      </StandardListSelection>

      <div className="flex flex-wrap gap-2">
        <ExpandablePanelTrigger
          aria-label="Section panel trigger hover"
          controlsID="overlay-hover-section-panel"
          data-testid="hover-panel-section"
          expanded={false}
          variant="section"
        >
          Section trigger
        </ExpandablePanelTrigger>
        <ExpandablePanelTrigger
          aria-label="Compact panel trigger hover"
          controlsID="overlay-hover-compact-panel"
          data-testid="hover-panel-compact"
          expanded={false}
          variant="compact"
        >
          Compact trigger
        </ExpandablePanelTrigger>
      </div>
    </section>
  );
}
