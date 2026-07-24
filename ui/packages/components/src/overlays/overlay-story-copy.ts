/** Static copy anchors for Storybook browser verification. Keep strings literal for bundle checks. */
export const OVERLAY_HOST_RESPONSIBILITY_DOCS =
  "Host apps supply accessible labels, trigger text, and overflow-safe content without dashboard providers.";

export const DIALOG_LONG_CONTENT_ANCHOR =
  "Dialog long content paragraph twenty for overflow review.";

export const DIALOG_CONTROLLED_ANCHOR =
  "Controlled dialog state is managed by the host application.";

export const DIALOG_ESCAPE_ANCHOR =
  "Press Escape to close this dialog and return focus to the trigger.";

export const DIALOG_MOBILE_ANCHOR =
  "Mobile dialog content remains reachable without horizontal page overflow.";

export const POPOVER_LONG_CONTENT_ANCHOR =
  "Popover long content paragraph twenty for overflow review.";

export const POPOVER_CONTROLLED_ANCHOR =
  "Controlled popover state is managed by the host application.";

export const POPOVER_KEYBOARD_OPEN_ANCHOR =
  "Popover opened from the keyboard trigger.";

export const POPOVER_VIEWPORT_PLACEMENT_ANCHOR =
  "Popover placement stays within the viewport near screen edges.";

export const COLLAPSIBLE_OPEN_ANCHOR =
  "Collapsible open state is visible to assistive technology.";

export const COLLAPSIBLE_CONTROLLED_ANCHOR =
  "Controlled collapsible state is managed by the host application.";

export const COLLAPSIBLE_NESTED_ANCHOR =
  "Nested collapsible inner section for disclosure review.";

export const SCROLL_AREA_HORIZONTAL_ANCHOR =
  "Wide horizontal scroll item twenty for overflow review.";

export const SCROLL_AREA_MOBILE_ANCHOR =
  "Mobile scroll area content remains reachable at narrow widths.";

export function createLongParagraphs(prefix: string, count: number): string[] {
  return Array.from({ length: count }, (_, index) => {
    const paragraphNumber = index + 1;
    return `${prefix} paragraph ${paragraphNumber}.`;
  });
}
