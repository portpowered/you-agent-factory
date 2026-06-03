import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "./dashboard-typography";

const CONTAINER_TEXT_SAMPLES = [
  {
    containerClass: "bg-primary-container",
    label: "on-primary-container",
    textClass: "text-on-primary-container",
  },
  {
    containerClass: "bg-success-container",
    label: "on-success-container",
    textClass: "text-on-success-container",
  },
] as const;

export function TypographyRoleHierarchyShowcase() {
  return (
    <section
      aria-label="Material typography and text color roles"
      className="grid max-w-3xl gap-8 rounded-2xl border border-outline bg-background p-6 text-on-surface"
    >
      <header className="grid gap-2">
        <h2 className={`m-0 ${DASHBOARD_PAGE_HEADING_CLASS}`}>
          Typography hierarchy (US-006)
        </h2>
        <p className={`m-0 max-w-2xl ${DASHBOARD_BODY_TEXT_CLASS}`}>
          Display and title roles carry wayfinding; body roles carry reading
          text; labels annotate controls. Text color roles stay separate from
          accent fills.
        </p>
      </header>

      <div className="grid gap-4">
        <h3 className={`m-0 ${DASHBOARD_SECTION_HEADING_CLASS}`}>
          Dashboard text scale
        </h3>
        <dl className="m-0 grid gap-3">
          <div className="grid gap-1">
            <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Page heading</dt>
            <dd className={`m-0 ${DASHBOARD_PAGE_HEADING_CLASS}`}>
              display / medium · on-surface
            </dd>
          </div>
          <div className="grid gap-1">
            <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
              Section heading
            </dt>
            <dd className={`m-0 ${DASHBOARD_SECTION_HEADING_CLASS}`}>
              title / large · on-surface
            </dd>
          </div>
          <div className="grid gap-1">
            <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Body</dt>
            <dd className={`m-0 ${DASHBOARD_BODY_TEXT_CLASS}`}>
              body / medium · on-surface-variant — long-form detail and table
              copy use this role for comfortable reading density.
            </dd>
          </div>
          <div className="grid gap-1">
            <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Supporting</dt>
            <dd className={`m-0 ${DASHBOARD_SUPPORTING_TEXT_CLASS}`}>
              body / small · on-surface-variant — chart axes, timestamps, and
              secondary metadata.
            </dd>
          </div>
          <div className="grid gap-1">
            <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Label</dt>
            <dd className={`m-0 ${DASHBOARD_SUPPORTING_LABEL_CLASS}`}>
              label / medium · on-surface-subtle
            </dd>
          </div>
          <div className="grid gap-1">
            <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Code extension</dt>
            <dd className={`m-0 ${DASHBOARD_BODY_CODE_CLASS}`}>
              code / medium · code — monospace beside Material body roles.
            </dd>
          </div>
        </dl>
      </div>

      <div className="grid gap-3">
        <h3 className={`m-0 ${DASHBOARD_SECTION_HEADING_CLASS}`}>
          Text on containers
        </h3>
        <div className="grid gap-2 sm:grid-cols-2">
          {CONTAINER_TEXT_SAMPLES.map(
            ({ containerClass, label, textClass }) => (
              <p
                key={label}
                className={`m-0 rounded-xl px-3 py-2 text-sm ${containerClass} ${textClass}`}
              >
                {label}
              </p>
            ),
          )}
        </div>
        <p className={`m-0 ${DASHBOARD_SUPPORTING_TEXT_CLASS}`}>
          <span className="text-on-surface-disabled">on-surface-disabled</span>
          {" · "}
          <span className="rounded-lg bg-primary px-2 py-0.5 text-on-inverse">
            on-inverse on primary fill
          </span>
        </p>
      </div>
    </section>
  );
}
