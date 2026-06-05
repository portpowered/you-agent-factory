import { Button } from "./button";
import { DashboardPanelShell } from "./dashboard-shell";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "./dashboard-typography";
import { Input } from "./input";
import {
  CardContentStack,
  DialogBodyLayout,
  FormGroupLayout,
  PageHeaderLayout,
  SectionStack,
  ToolbarRowLayout,
} from "./layout-primitives";

export function LayoutRoleShowcase() {
  return (
    <section
      aria-label="Material layout spacing primitives"
      className="grid max-w-3xl gap-layout-page rounded-2xl border border-outline bg-background p-layout-inset-dialog text-on-surface"
    >
      <header className="grid gap-layout-tight">
        <h2 className={`m-0 ${DASHBOARD_PAGE_HEADING_CLASS}`}>
          Layout spacing (US-007)
        </h2>
        <p className={`m-0 max-w-2xl ${DASHBOARD_BODY_TEXT_CLASS}`}>
          Shared primitives encode recurring page, card, toolbar, form, and
          dialog rhythms using layout spacing roles on a 4px grid.
        </p>
      </header>

      <SectionStack>
        <PageHeaderLayout
          actions={
            <Button size="sm" type="button">
              Action
            </Button>
          }
          heading={
            <>
              <h3 className={`m-0 ${DASHBOARD_SECTION_HEADING_CLASS}`}>
                Page header layout
              </h3>
              <p className={`m-0 ${DASHBOARD_BODY_TEXT_CLASS}`}>
                Title block with trailing actions uses gap-layout-group.
              </p>
            </>
          }
        />

        <DashboardPanelShell as="article" className="grid gap-layout-group" inset>
          <h3 className={`m-0 ${DASHBOARD_SECTION_HEADING_CLASS}`}>
            Toolbar row layout
          </h3>
          <ToolbarRowLayout>
            <Button size="sm" tone="outline" type="button">
              Filter
            </Button>
            <Button size="sm" tone="outline" type="button">
              Export
            </Button>
            <Button size="sm" type="button">
              Primary action
            </Button>
          </ToolbarRowLayout>
        </DashboardPanelShell>

        <DashboardPanelShell as="article" className="grid gap-layout-group" inset>
          <h3 className={`m-0 ${DASHBOARD_SECTION_HEADING_CLASS}`}>
            Stacked card layout
          </h3>
          <CardContentStack>
            <p className={`m-0 ${DASHBOARD_BODY_TEXT_CLASS}`}>
              Card interiors stack related blocks at gap-layout-element.
            </p>
            <p className={`m-0 ${DASHBOARD_BODY_TEXT_CLASS}`}>
              Section stacks separate major groups at gap-layout-section.
            </p>
          </CardContentStack>
        </DashboardPanelShell>

        <DashboardPanelShell as="article" className="grid gap-layout-group" inset>
          <h3 className={`m-0 ${DASHBOARD_SECTION_HEADING_CLASS}`}>
            Form / dialog body layout
          </h3>
          <DialogBodyLayout>
            <FormGroupLayout>
              <label
                className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                htmlFor="layout-demo-field"
              >
                Field label
              </label>
              <Input
                id="layout-demo-field"
                placeholder="gap-layout-tight within group"
              />
            </FormGroupLayout>
            <p className={`m-0 ${DASHBOARD_BODY_TEXT_CLASS}`}>
              Dialog bodies use gap-layout-group between header, fields, and
              footer regions.
            </p>
          </DialogBodyLayout>
        </DashboardPanelShell>
      </SectionStack>
    </section>
  );
}
