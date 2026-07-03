# Form input primitives

Domain-free text input, textarea, checkbox, and file input controls for reusable
forms. Use these primitives when you need accessible native form semantics with
package styling, without importing dashboard features, API clients, or app state.

These components are **presentation controls**. They render styled native inputs
and forward standard React form props. They do not fetch data, perform validation,
upload files, or own durable application state.

## Required setup

Import package styles once in your host application before rendering primitives:

```css
@import "@you-agent-factory/components/styles.css";
```

Primitives depend on package role tokens and Tailwind utility classes compiled
from those tokens. They do not require dashboard `styles.css`, dashboard providers,
generated OpenAPI types, React Query, Zustand, or app localization context.

## Import paths

Prefer the forms category entrypoint for tree-shaking and explicit category
boundaries:

```ts
import {
  PackageCheckbox,
  PackageFileInput,
  PackageInput,
  PackageTextarea,
  inputVariants,
  textareaVariants,
} from "@you-agent-factory/components/forms";
```

The same exports are also available from the package root:

```ts
import { PackageInput } from "@you-agent-factory/components";
```

## Host ownership

The package owns presentation markup, focus rings, disabled styling, and native
control semantics. Host applications own everything domain-specific:

| Concern | Owner |
| --- | --- |
| Visible labels and placeholder copy | Host |
| Helper text and validation error messages | Host |
| `id`, `name`, `required`, and `disabled` values | Host |
| `aria-describedby`, `aria-invalid`, and `aria-label` wiring | Host |
| Controlled value or checked state | Host (when using controlled mode) |
| Form submission, validation rules, and error mapping | Host |
| File upload, persistence, and storage side effects | Host |
| Selected-file display copy (for example helper text) | Host |
| Data fetching, caching, and durable cross-route state | Host |

Pass values, handlers, and accessibility attributes into primitives as props.
Do not expect primitives to read from your routers, stores, or API layers.

## Controlled versus uncontrolled values

All four primitives forward native React form props. Choose the mode that matches
how your host form manages state.

### Text input (`PackageInput`)

| Mode | When to use | Key props |
| --- | --- | --- |
| Controlled | Host state must mirror every keystroke (live validation, dependent fields, reset) | `value` + `onChange` |
| Uncontrolled | Native form submission or refs are enough; host does not need per-keystroke state | `defaultValue` + optional `ref` |

```tsx
import { useState } from "react";
import { PackageInput } from "@you-agent-factory/components/forms";

export function ControlledNameField() {
  const [name, setName] = useState("");

  return (
    <PackageInput
      aria-label="Display name"
      onChange={(event) => {
        setName(event.target.value);
      }}
      value={name}
    />
  );
}

export function UncontrolledNameField() {
  return (
    <PackageInput
      aria-label="Display name"
      defaultValue="Initial name"
      name="displayName"
    />
  );
}
```

Do not pass both `value` and `defaultValue` on the same control.

### Textarea (`PackageTextarea`)

Same controlled/uncontrolled rules as text input. `PackageTextarea` also accepts
`variant`:

- `field` (default) — bordered field shell with scrollable body (`textareaVariants`).
- `plain` — borderless transparent textarea for embedding inside custom shells.

### Checkbox (`PackageCheckbox`)

| Mode | When to use | Key props |
| --- | --- | --- |
| Controlled | Host must react to every toggle (conditional sections, select-all logic) | `checked` + `onChange` |
| Uncontrolled | Checkbox state can live in the native form until submit | `defaultChecked` + optional `ref` |

The visible check indicator is decorative (`aria-hidden`). Assistive technology
reads the native `input type="checkbox"` inside the component. Always provide an
accessible name with a visible `<label htmlFor={id}>`, `aria-label`, or
`aria-labelledby`.

### File input (`PackageFileInput`)

| Mode | When to use | Key props |
| --- | --- | --- |
| Controlled selection tracking | Host must show selected-file feedback or gate submit buttons | Read `event.target.files` in `onChange`; optionally clear with `value=""` on re-render |
| Uncontrolled | File selection is handled entirely by native form submit | `name` on the input; read files from `FormData` on submit |

The primitive opens the native file picker and exposes selected `File` objects
through standard `onChange` events. It does **not** upload, persist, or validate
file contents. Display selected file names in host-owned helper text or adjacent
copy when your UX requires feedback.

```tsx
import { useState } from "react";
import { PackageFileInput } from "@you-agent-factory/components/forms";

export function CoverImageField() {
  const [selectedName, setSelectedName] = useState<string | undefined>();

  return (
    <>
      <PackageFileInput
        accept="image/png,image/jpeg"
        aria-describedby="cover-image-helper"
        id="cover-image"
        onChange={(event) => {
          setSelectedName(event.target.files?.[0]?.name);
        }}
      />
      <p id="cover-image-helper">
        {selectedName ? `Selected: ${selectedName}` : "PNG or JPEG up to 2 MB."}
      </p>
    </>
  );
}
```

## Labels, descriptions, and error relationships

Primitives render the control only. Host applications wire labels and supporting
text:

1. Generate stable `id` values (for example `useId()`).
2. Associate visible labels with `<label htmlFor={controlId}>`.
3. Render helper text in an element with its own `id`.
4. Render error text in an element with `role="alert"` and its own `id`.
5. Pass `aria-describedby` listing helper and/or error element ids.
6. Pass `aria-invalid={true}` when the field fails validation.

```tsx
import { useId } from "react";
import { PackageInput } from "@you-agent-factory/components/forms";

export function NameField({
  errorText,
  helperText,
  invalid,
  value,
  onChange,
}: {
  errorText?: string;
  helperText?: string;
  invalid?: boolean;
  value: string;
  onChange: (value: string) => void;
}) {
  const reactId = useId();
  const controlId = `${reactId}-control`;
  const helperId = helperText ? `${reactId}-helper` : undefined;
  const errorId = errorText ? `${reactId}-error` : undefined;
  const describedBy =
    [helperId, errorId].filter(Boolean).join(" ") || undefined;

  return (
    <div>
      <label htmlFor={controlId}>Display name</label>
      <PackageInput
        aria-describedby={describedBy}
        aria-invalid={invalid || Boolean(errorText) || undefined}
        id={controlId}
        onChange={(event) => {
          onChange(event.target.value);
        }}
        value={value}
      />
      {helperText ? <p id={helperId}>{helperText}</p> : null}
      {errorText ? (
        <p id={errorId} role="alert">
          {errorText}
        </p>
      ) : null}
    </div>
  );
}
```

Package Storybook uses `PackageFormStoryField` for the same wiring pattern in
demos. That helper is story support only; host applications should compose
labels and messages in their own form-field layer.

## Required, disabled, and invalid state

| State | Host responsibility | Primitive behavior |
| --- | --- | --- |
| `required` | Set `required` when the field must be submitted | Native constraint validation semantics |
| `disabled` | Set `disabled` to block interaction | Pointer and keyboard changes are ignored; disabled styling and semantics apply |
| Invalid | Set `aria-invalid={true}` when validation fails | Danger border and focus ring via `aria-invalid` styles |
| Error copy | Render error text and link with `aria-describedby` | Does not invent error messages |

Disabled file inputs ignore `onChange` and keep an empty `files` list until
re-enabled.

## Accessibility expectations

- Text input, textarea, and file input expose visible `focus-visible` rings for
  keyboard users.
- Checkbox keyboard focus targets the native input; the styled indicator shows a
  focus ring via `peer-focus-visible` styles.
- Invalid states use `aria-invalid` styling on the native control (checkbox uses
  `peer-aria-invalid` on the indicator).
- Host-supplied header actions, labels, and error text must use accessible names
  and programmatic relationships (`htmlFor`, `aria-describedby`, `role="alert"`).
- Do not rely on placeholder text as the only accessible name.

## Variant helpers

`inputVariants` and `textareaVariants` export the shared field class strings for
host compositions that need matching shells around sibling elements:

```ts
import { inputVariants, textareaVariants } from "@you-agent-factory/components/forms";
```

## Storybook visual reference

Package Storybook lives under `Forms/PackageInput`, `Forms/PackageTextarea`,
`Forms/PackageCheckbox`, and `Forms/PackageFileInput`. Stories use package
imports and package token decorators only — no dashboard providers.

| Primitive | Example stories |
| --- | --- |
| `PackageInput` | Controlled, Uncontrolled, Disabled, Invalid, ErrorState, Focus, HelperText, MobileWidth |
| `PackageTextarea` | Controlled, Uncontrolled, Disabled, Invalid, ErrorState, Focus, HelperText, MobileWidth |
| `PackageCheckbox` | Controlled, Uncontrolled, Disabled, Invalid, ErrorState, Focus, HelperText, MobileWidth |
| `PackageFileInput` | Default, Disabled, Invalid, ErrorState, Focus, HelperText, SelectedFile, MobileWidth |

Run package Storybook locally:

```bash
cd ui/packages/components
bun run storybook
```

Browser verification for responsive and focus stories:

```bash
cd ui/packages/components
bun run verify:storybook-browser
```

## Allowed dependencies

Primitive source may import:

- Package utilities (`cn` from `@you-agent-factory/components/utilities`)
- Package token CSS via the host `styles.css` import
- React and `react-dom` peer dependencies

Primitive source must **not** import:

- Dashboard feature modules, routes, or providers
- Generated OpenAPI clients or dashboard API adapters
- React Query, Zustand, Monaco, Sonner, or dashboard i18n/session providers
- Factory, work, session, or provider domain types

`check:package-boundary` enforces these rules in CI.

## Dashboard integration note

The dashboard re-exports these primitives from `components/ui` (`Input`,
`Textarea`, `Checkbox`, `FileInput`) so existing feature imports keep working
while resolving to package implementations. New host applications should import
directly from `@you-agent-factory/components` or
`@you-agent-factory/components/forms`.
