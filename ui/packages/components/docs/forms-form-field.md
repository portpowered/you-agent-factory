# Form field structure and messaging

Domain-free form-field layout and messaging components for reusable forms. Use
these components when you need consistent label, description, helper text,
warning, error, and success presentation with package typography — without
importing dashboard features, API clients, generated OpenAPI types, or app
state.

Form-field components are **presentation-only**. They render styled labels and
message regions and forward standard React props. They do not fetch data,
perform validation decisions, submit forms, or own durable application state.

## Required setup

Import package styles once in your host application before rendering form fields:

```css
@import "@you-agent-factory/components/styles.css";
```

Form-field messaging depends on package role tokens and Tailwind utility classes
compiled from those tokens. It does not require dashboard `styles.css`, dashboard
providers, generated OpenAPI types, React Query, Zustand, or app localization
context.

## Import paths

Prefer the forms category entrypoint for tree-shaking and explicit category
boundaries:

```ts
import {
  buildFormFieldAriaDescribedBy,
  FormDescription,
  FormError,
  FormField,
  FormFieldGroup,
  FormFieldGroupLabel,
  FormHelperText,
  FormLabel,
  FormSuccess,
  FormWarning,
  type FormFieldMessageIds,
} from "@you-agent-factory/components/forms";
```

The same exports are also available from the package root:

```ts
import {
  FormField,
  FormLabel,
  buildFormFieldAriaDescribedBy,
} from "@you-agent-factory/components";
```

Pair form-field messaging with package input primitives (`PackageInput`,
`PackageTextarea`, `PackageCheckbox`, `NativeSelect`, `Select`, and related
controls). See [Form input primitives](./forms-input-primitives.md) and
[Form select primitives](./forms-select-primitives.md) for control-specific
behavior.

## Component overview

| Component | Role |
| --- | --- |
| `FormField` | Vertical spacing wrapper for a single control and its messaging |
| `FormLabel` | Visible label associated with a control via `htmlFor` |
| `FormDescription` | Supporting copy below the label (body or supporting typography) |
| `FormHelperText` | Persistent guidance below the control |
| `FormWarning` | Cautionary message — no default `role`; pass `status` or `alert` when live-region announcement is desired |
| `FormError` | Validation error message (defaults to `role="alert"`) |
| `FormSuccess` | Positive confirmation (defaults to `role="status"`) |
| `FormFieldGroup` | `fieldset` wrapper for grouped controls |
| `FormFieldGroupLabel` | `legend` for a grouped field |
| `buildFormFieldAriaDescribedBy` | Joins host-supplied message element ids into one `aria-describedby` value |

## Host ownership

The package owns presentation markup, typography tokens, and message-region
semantics. Host applications own everything domain-specific:

| Concern | Owner |
| --- | --- |
| Visible labels, descriptions, and placeholder copy | Host |
| Helper text, warning text, error text, and success text | Host |
| `id`, `name`, `required`, `disabled`, and `aria-invalid` on controls | Host |
| `aria-describedby`, `aria-errormessage`, and `aria-labelledby` wiring | Host |
| Controlled values, checked state, and change handlers | Host |
| Validation rules, validation state transitions, and error mapping | Host |
| Form submission, persistence, and side effects | Host |
| Required affordances in label copy (for example `*` or “required”) | Host |
| Data fetching, caching, and durable cross-route state | Host |

Pass values, handlers, message copy, and accessibility attributes into
components as props. Do not expect form-field components to read from your
routers, stores, or API layers.

## Accessibility relationships

Form-field components render message regions with stable `id` values that hosts
wire to controls. The package does **not** automatically set `aria-describedby`
or `aria-errormessage` on controls — hosts must pass those attributes to input
primitives.

### Label and accessible name

Associate a visible label with the control:

```tsx
<FormLabel htmlFor={controlId}>Display name</FormLabel>
<PackageInput id={controlId} />
```

Assistive technology reads the accessible name from the label text. For controls
without a visible label, supply `aria-label` or `aria-labelledby` on the control
instead.

### Description, helper, warning, and success

Include message element ids in `aria-describedby` on the control when those
messages are present. Use `buildFormFieldAriaDescribedBy` to join ids:

```ts
const ariaDescribedBy = buildFormFieldAriaDescribedBy({
  descriptionId,
  helperId,
  warningId,
  successId,
});
```

Omit ids for messages that are not rendered. When no messages are supplied,
omit `aria-describedby` entirely.

| Message type | Typical `role` | Included in `aria-describedby` |
| --- | --- | --- |
| `FormDescription` | (none) | Yes, when rendered |
| `FormHelperText` | (none) | Yes, when rendered |
| `FormWarning` | `status` or `alert` (no default — host supplies when needed) | Yes, when rendered |
| `FormSuccess` | `status` (default) | Yes, when rendered |
| `FormError` | `alert` (default) | Host choice — see error section |

### Error text and invalid state

Expose validation failures without relying on color alone:

1. Set `aria-invalid={true}` on the control when the field fails validation.
2. Render error copy in `FormError` with a stable `id`.
3. Connect the error to the control with **either**:
   - `aria-errormessage={errorId}` (recommended when error text should not
     duplicate the accessible description), or
   - include `errorId` in `aria-describedby` via `buildFormFieldAriaDescribedBy`.

When using `aria-errormessage`, omit `errorId` from
`buildFormFieldAriaDescribedBy` so error copy is not repeated in the accessible
description.

`FormError` defaults to `role="alert"` so assistive technology announces new
validation failures.

### Warning text role

`FormWarning` does not set a default `role`. Pass `role="status"` for
persistent cautionary copy that should be discoverable through
`aria-describedby`, or `role="alert"` when assistive technology should announce
new warning text immediately. Omit `role` when the warning is purely visual
supplement to description or helper text.

### Required and disabled state

| State | Host responsibility | Package behavior |
| --- | --- | --- |
| Required | Set `required` on the native control; add visible required affordance in label copy when your UX requires it | `FormLabel` renders host-supplied label content only |
| Disabled | Set `disabled` on the native control | Message components remain readable; disabled styling applies on the control |
| Invalid | Set `aria-invalid={true}` when validation fails | `FormError` uses error-container typography tokens |

Required affordances such as asterisks or “(required)” are host-supplied label
content. Native `required` and `disabled` attributes on controls expose state to
assistive technology.

### Grouped controls

Use `FormFieldGroup` and `FormFieldGroupLabel` for related controls that share
group context:

```tsx
<FormFieldGroup aria-describedby={groupDescriptionId}>
  <FormFieldGroupLabel>Notification preferences</FormFieldGroupLabel>
  <FormDescription id={groupDescriptionId}>
    Choose how you want to receive updates.
  </FormDescription>
  <FormField>
    <FormLabel htmlFor={emailId}>Email</FormLabel>
    <PackageCheckbox id={emailId} />
  </FormField>
  <FormField>
    <FormLabel htmlFor={smsId}>Text message</FormLabel>
    <PackageCheckbox id={smsId} />
  </FormField>
</FormFieldGroup>
```

`FormFieldGroup` renders a borderless `fieldset`. The group label renders as a
`legend`. Per-control labels still use `FormLabel` with `htmlFor` pointing at
each control id. Shared group description ids can be referenced from
`aria-describedby` on the `fieldset`.

## Minimal examples

### Text field with label and helper text

```tsx
import { useId } from "react";
import {
  buildFormFieldAriaDescribedBy,
  FormField,
  FormHelperText,
  FormLabel,
} from "@you-agent-factory/components/forms";
import { PackageInput } from "@you-agent-factory/components/forms";

export function DisplayNameField() {
  const reactId = useId();
  const controlId = `${reactId}-control`;
  const helperId = `${reactId}-helper`;

  return (
    <FormField>
      <FormLabel htmlFor={controlId}>Display name</FormLabel>
      <PackageInput
        aria-describedby={buildFormFieldAriaDescribedBy({ helperId })}
        id={controlId}
      />
      <FormHelperText id={helperId}>Use 3 to 40 characters.</FormHelperText>
    </FormField>
  );
}
```

### Required disabled field

```tsx
import { useId } from "react";
import {
  FormField,
  FormLabel,
} from "@you-agent-factory/components/forms";
import { PackageInput } from "@you-agent-factory/components/forms";

export function LockedDisplayNameField() {
  const reactId = useId();
  const controlId = `${reactId}-control`;

  return (
    <FormField>
      <FormLabel htmlFor={controlId}>
        Display name
        <span className="text-on-error-container"> *</span>
      </FormLabel>
      <PackageInput disabled id={controlId} required value="Example value" />
    </FormField>
  );
}
```

### Error field with `aria-errormessage`

```tsx
import { useId } from "react";
import {
  buildFormFieldAriaDescribedBy,
  FormError,
  FormField,
  FormLabel,
} from "@you-agent-factory/components/forms";
import { PackageInput } from "@you-agent-factory/components/forms";

export function InvalidDisplayNameField({ errorText }: { errorText: string }) {
  const reactId = useId();
  const controlId = `${reactId}-control`;
  const errorId = `${reactId}-error`;

  return (
    <FormField>
      <FormLabel htmlFor={controlId}>Display name</FormLabel>
      <PackageInput
        aria-describedby={buildFormFieldAriaDescribedBy({})}
        aria-errormessage={errorId}
        aria-invalid
        id={controlId}
      />
      <FormError id={errorId}>{errorText}</FormError>
    </FormField>
  );
}
```

### Warning or success field

```tsx
import { useId } from "react";
import {
  buildFormFieldAriaDescribedBy,
  FormField,
  FormLabel,
  FormSuccess,
  FormWarning,
} from "@you-agent-factory/components/forms";
import { PackageInput } from "@you-agent-factory/components/forms";

export function StatusDisplayNameField({
  successText,
  warningText,
}: {
  successText?: string;
  warningText?: string;
}) {
  const reactId = useId();
  const controlId = `${reactId}-control`;
  const warningId = warningText ? `${reactId}-warning` : undefined;
  const successId = successText ? `${reactId}-success` : undefined;

  return (
    <FormField>
      <FormLabel htmlFor={controlId}>Display name</FormLabel>
      <PackageInput
        aria-describedby={buildFormFieldAriaDescribedBy({
          warningId,
          successId,
        })}
        id={controlId}
      />
      {warningText ? (
        <FormWarning id={warningId} role="status">
          {warningText}
        </FormWarning>
      ) : null}
      {successText ? (
        <FormSuccess id={successId} role="status">
          {successText}
        </FormSuccess>
      ) : null}
    </FormField>
  );
}
```

### Grouped-control field

```tsx
import { useId } from "react";
import {
  FormDescription,
  FormField,
  FormFieldGroup,
  FormFieldGroupLabel,
  FormLabel,
} from "@you-agent-factory/components/forms";
import { PackageCheckbox } from "@you-agent-factory/components/forms";

export function NotificationPreferencesField() {
  const reactId = useId();
  const descriptionId = `${reactId}-description`;
  const emailId = `${reactId}-email`;
  const smsId = `${reactId}-sms`;

  return (
    <FormFieldGroup aria-describedby={descriptionId}>
      <FormFieldGroupLabel>Notification preferences</FormFieldGroupLabel>
      <FormDescription id={descriptionId}>
        Choose how you want to receive updates.
      </FormDescription>
      <FormField>
        <FormLabel htmlFor={emailId}>Email</FormLabel>
        <PackageCheckbox defaultChecked id={emailId} />
      </FormField>
      <FormField>
        <FormLabel htmlFor={smsId}>Text message</FormLabel>
        <PackageCheckbox id={smsId} />
      </FormField>
    </FormFieldGroup>
  );
}
```

## Host-owned validation updates

Hosts update validation state and message props after user interaction. The
rendered accessible relationships must follow the new state:

```tsx
import { useId, useState } from "react";
import {
  buildFormFieldAriaDescribedBy,
  FormError,
  FormField,
  FormLabel,
} from "@you-agent-factory/components/forms";
import { PackageInput } from "@you-agent-factory/components/forms";

export function ValidatedDisplayNameField() {
  const reactId = useId();
  const controlId = `${reactId}-control`;
  const errorId = `${reactId}-error`;
  const [value, setValue] = useState("");
  const [errorText, setErrorText] = useState<string | undefined>();

  return (
    <FormField>
      <FormLabel htmlFor={controlId}>Display name</FormLabel>
      <PackageInput
        aria-describedby={buildFormFieldAriaDescribedBy({})}
        aria-errormessage={errorText ? errorId : undefined}
        aria-invalid={errorText ? true : undefined}
        id={controlId}
        onChange={(event) => {
          const nextValue = event.target.value;
          setValue(nextValue);
          setErrorText(
            nextValue.trim().length === 0 ? "Display name is required." : undefined,
          );
        }}
        value={value}
      />
      {errorText ? <FormError id={errorId}>{errorText}</FormError> : null}
    </FormField>
  );
}
```

Form-field components render caller-provided message text exactly as supplied.
They do not generate validation copy or decide when a field is valid.

## Storybook visual reference

Package Storybook lives under `Forms/PackageFormField`. Stories use package
imports and domain-neutral copy only — no dashboard providers.

| Story | What it shows |
| --- | --- |
| `Default` | Label and control only |
| `Required` | Required affordance in label copy |
| `Disabled` | Disabled control with label |
| `HelperText` | Helper text in accessible description |
| `Warning` | Warning message styling and description wiring |
| `ErrorState` | `aria-invalid`, error copy, and `aria-errormessage` |
| `Success` | Success message styling |
| `LongMessage` | Long helper text at narrow widths |
| `GroupedControl` | Fieldset, legend, shared description, per-control labels |
| `Focus` | Keyboard focus treatment on the control |
| `MobileWidth` | 320px viewport layout |

Run package Storybook locally:

```bash
cd ui/packages/components
bun run storybook
```

Browser verification for form-field responsive and focus stories:

```bash
cd ui/packages/components
bun run verify:form-field-storybook-browser
```

## Allowed dependencies

Form-field source may import:

- Package utilities (`cn` from `@you-agent-factory/components/utilities`)
- Package token CSS via the host `styles.css` import
- React and `react-dom` peer dependencies

Form-field source must **not** import:

- Dashboard feature modules, routes, or providers
- Generated OpenAPI clients or dashboard API adapters
- React Query, Zustand, Monaco, Sonner, or dashboard i18n/session providers
- Factory, work, session, or provider domain types

`check:package-boundary` enforces these rules in CI.

## Dashboard integration note

The dashboard re-exports form-field messaging from `components/ui` so existing
feature imports keep working while resolving to package implementations. New host
applications should import directly from `@you-agent-factory/components` or
`@you-agent-factory/components/forms`.
