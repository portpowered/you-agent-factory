# Form select primitives

Domain-free select controls for reusable forms. Use these primitives when you need
accessible combobox or native `<select>` semantics with package styling, without
importing dashboard features, API clients, generated OpenAPI types, or app
localization providers.

The package ships three layers:

| Component | Role |
| --- | --- |
| `Select` primitives | Radix-backed combobox building blocks (`Select`, `SelectTrigger`, `SelectContent`, `SelectItem`, and helpers). |
| `NativeSelect` | Styled native `<select>` for simple option lists and progressive enhancement. |
| `EnumSelect` helpers | Opinionated wrappers that map host-supplied option arrays into `Select` markup. |

These components are **presentation controls**. They render styled select UI and
forward standard React props. They do not fetch options, perform validation,
localize labels, or own durable application state.

## Required setup

Import package styles once in your host application before rendering selects:

```css
@import "@you-agent-factory/components/styles.css";
```

Select primitives depend on package role tokens and Tailwind utility classes
compiled from those tokens. They do not require dashboard `styles.css`, dashboard
providers, generated OpenAPI types, React Query, Zustand, or app localization
context.

## Import paths

Prefer the forms category entrypoint for tree-shaking and explicit category
boundaries:

```ts
import {
  EnumSelect,
  ENUM_SELECT_EMPTY_VALUE,
  NativeSelect,
  OptionalEnumSelect,
  ResetEnumSelect,
  Select,
  SELECT_EMPTY_STATE_VALUE,
  SelectContent,
  SelectEmpty,
  SelectField,
  SelectItem,
  SelectTrigger,
  SelectValue,
  type EnumSelectOption,
} from "@you-agent-factory/components/forms";
```

The same exports are also available from the package root:

```ts
import {
  EnumSelect,
  NativeSelect,
  Select,
} from "@you-agent-factory/components";
```

## Host ownership

The package owns presentation markup, focus rings, disabled styling, keyboard
interaction, and combobox semantics. Host applications own everything
domain-specific:

| Concern | Owner |
| --- | --- |
| Option values and visible labels | Host |
| Placeholder, empty, and loading copy | Host |
| Visible labels, helper text, and validation errors | Host |
| `id`, `disabled`, and `aria-*` wiring | Host |
| Controlled `value` and `open` state | Host (when using controlled mode) |
| `onValueChange` and `onOpenChange` handlers | Host |
| Fetching, caching, and refreshing option lists | Host |
| Localization and enum label mapping | Host |
| Form submission, validation rules, and error mapping | Host |

Pass data and callbacks into select components as props. Do not expect the
package to read from your routers, stores, or API layers.

## Option contract

### `EnumSelectOption`

`EnumSelect`, `OptionalEnumSelect`, and `ResetEnumSelect` accept a readonly
array of domain-free options:

```ts
type EnumSelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};
```

| Field | Contract |
| --- | --- |
| `value` | Stable string identifier used in `onValueChange`. Must be unique within the option list. |
| `label` | Host-supplied visible text. The package does not translate or format enum values. |
| `disabled` | When `true`, the option is not selectable by pointer or keyboard and exposes disabled semantics to assistive technology. |

### Empty option lists

When `options` is empty and `loading` is `false`, `EnumSelect` renders a
non-selectable `SelectEmpty` row using `emptyOptionsLabel` (default:
`"No options available"`). The row is a disabled menu item backed by
`SELECT_EMPTY_STATE_VALUE`, so users cannot accidentally select a fake value.

For custom Radix compositions, render `SelectEmpty` inside `SelectContent`
instead of mapping interactive `SelectItem` rows over an empty array.

### Optional and reset helpers

| Helper | Use when |
| --- | --- |
| `OptionalEnumSelect` | The host needs a nullable selection. Uses `ENUM_SELECT_EMPTY_VALUE` as a dedicated empty row and maps it back to `null` in `onValueChange`. |
| `ResetEnumSelect` | The host needs a select that always returns to the placeholder after each selection (for example one-shot filters). Remounts internally after each change. |

## Controlled value and open state

`Select` forwards Radix `Select.Root` props. Choose controlled or uncontrolled
mode based on how your host form manages state.

### Controlled value

Use controlled value when host state must mirror the current selection (dependent
fields, validation, reset, or async reconciliation):

```tsx
import { useState } from "react";
import {
  EnumSelect,
  type EnumSelectOption,
} from "@you-agent-factory/components/forms";

const options: EnumSelectOption[] = [
  { label: "Story", value: "story" },
  { label: "Task", value: "task" },
];

function WorkTypeField() {
  const [value, setValue] = useState("story");

  return (
    <EnumSelect
      id="work-type"
      onValueChange={setValue}
      options={options}
      placeholder="Select a work type"
      value={value}
    />
  );
}
```

Host responsibilities for controlled value:

- Provide the current `value` on every render.
- Update host state inside `onValueChange` when the user selects a different
  **enabled** option.
- Expect `onValueChange` not to fire when the user re-selects the current value
  or activates a disabled option.

### Controlled open

Use controlled open when the host must coordinate menu visibility with other UI
(for example closing on route change or synchronizing with a parent disclosure):

```tsx
import { useState } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@you-agent-factory/components/forms";

function ControlledOpenExample() {
  const [open, setOpen] = useState(false);

  return (
    <Select onOpenChange={setOpen} open={open} value="story">
      <SelectTrigger aria-label="Work type">
        <SelectValue placeholder="Select a work type" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="story">Story</SelectItem>
      </SelectContent>
    </Select>
  );
}
```

Host responsibilities for controlled open:

- Provide the current `open` boolean on every render.
- Update host state inside `onOpenChange` when the user opens or closes the
  menu.

### Uncontrolled usage

Omit `value` and `open` to let Radix manage internal state. `defaultValue` still
sets the initial selection. Prefer controlled mode when host validation or
cross-field logic depends on the current selection.

## Keyboard behavior and focus

Radix select primitives provide combobox semantics on the trigger:

| Key | Behavior |
| --- | --- |
| `Tab` / `Shift+Tab` | Move focus to and from the trigger. |
| `Space`, `Enter`, `ArrowDown`, `ArrowUp` | Open the menu when closed. |
| `ArrowDown` / `ArrowUp` | Move highlight across enabled options while open. |
| `Enter` / `Space` | Select the highlighted enabled option and close the menu. |
| `Escape` | Close the menu without changing the selection. |
| `Home` / `End` | Jump to the first or last enabled option while open. |

After a successful selection, focus returns to the trigger. Package styles keep
`focus-visible` rings on the trigger and highlighted menu items during keyboard
interaction.

`NativeSelect` uses native platform keyboard behavior for `<select>` elements.

## Disabled behavior

| Surface | Behavior |
| --- | --- |
| Disabled field (`disabled` on `Select` or `EnumSelect`) | Trigger is not interactive, exposes disabled semantics, and cannot open the menu. |
| Disabled option (`disabled` on `SelectItem` or `EnumSelectOption`) | Skipped by keyboard navigation and not selectable by pointer. |
| Loading `EnumSelect` (`loading`) | Trigger is disabled, sets `aria-busy`, and shows `loadingLabel` as the placeholder. Controlled value is routed through `SELECT_EMPTY_STATE_VALUE` so the loading label stays visible. |

## Loading and empty states

`EnumSelect` supports asynchronous option loading without dashboard wiring:

```tsx
<EnumSelect
  id="work-type"
  loading={isLoading}
  loadingLabel="Loading work types..."
  emptyOptionsLabel="No work types available"
  onValueChange={setValue}
  options={options}
  value={value}
/>
```

While `loading` is `true`:

- The trigger is disabled and marked busy.
- The menu shows a non-selectable `SelectEmpty` row with `loadingLabel`.
- Host `value` is not shown in the trigger; the loading label takes precedence.

When loading completes and `options` is still empty, `emptyOptionsLabel` is used
for both the menu affordance and the visible empty state.

## Error and description relationships

Use host-owned helper and error text with explicit ARIA relationships on the
trigger. `SelectField` is a small layout helper when you want label, description,
and error regions colocated with the control:

```tsx
import {
  Select,
  SelectContent,
  SelectField,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@you-agent-factory/components/forms";

function WorkTypeField({ error }: { error?: string }) {
  const controlId = "work-type";
  const helperId = "work-type-helper";
  const errorId = "work-type-error";

  return (
    <SelectField
      description="Choose the work type for this request."
      descriptionId={helperId}
      error={error}
      errorId={errorId}
      inputId={controlId}
      label="Work type"
    >
      <Select value="story">
        <SelectTrigger
          aria-describedby={`${helperId} ${errorId}`}
          aria-invalid={Boolean(error)}
          id={controlId}
        >
          <SelectValue placeholder="Select a work type" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="story">Story</SelectItem>
        </SelectContent>
      </Select>
    </SelectField>
  );
}
```

Error text renders with `role="alert"` so assistive technology announces
validation failures without relying on color alone. Wire `aria-describedby` to
both helper and error element ids when both are present.

## Long labels and compact layouts

`SelectTrigger` clamps the selected value with `line-clamp-1`. `SelectItem` text
uses `line-clamp-2` with `break-words` so long option labels stay inside the
menu without overlapping neighboring UI. Prefer concise labels in product copy,
but the package constrains overflow for narrow containers and mobile widths.

## NativeSelect

Use `NativeSelect` when a native `<select>` is acceptable and you only need
package field styling:

```tsx
import { NativeSelect } from "@you-agent-factory/components/forms";

function PriorityField() {
  return (
    <NativeSelect aria-label="Priority" defaultValue="medium" id="priority">
      <option value="low">Low</option>
      <option value="medium">Medium</option>
      <option value="high">High</option>
    </NativeSelect>
  );
}
```

`NativeSelect` forwards standard `<select>` props. Host applications still own
`<option>` children, labels, validation messaging, and change handlers.

## Storybook examples

Package Storybook stories demonstrate expected states with package imports only.
Run `bun run storybook` from `ui/packages/components` or inspect the static
build with `bun run build-storybook`.

| Story | What it shows |
| --- | --- |
| `Forms/PackageSelect/ControlledValue` | Host-controlled selection |
| `Forms/PackageSelect/ControlledOpen` | Host-controlled menu open state |
| `Forms/PackageSelect/KeyboardInteraction` | Keyboard open, navigate, select, and focus return |
| `Forms/PackageSelect/DisabledField` | Disabled trigger |
| `Forms/PackageSelect/EmptyOptions` | Non-selectable empty menu affordance |
| `Forms/PackageSelect/LoadingOptions` | Loading trigger and menu state |
| `Forms/PackageSelect/ErrorState` | `aria-invalid` and visible error text |
| `Forms/PackageSelect/LongLabel` | Long labels at desktop width |
| `Forms/PackageSelect/LongLabelMobile` | Long labels at mobile width |
