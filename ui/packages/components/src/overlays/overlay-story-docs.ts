import { OVERLAY_HOST_RESPONSIBILITY_DOCS } from "./overlay-story-copy";

export const overlayStoryDocs = {
  component: OVERLAY_HOST_RESPONSIBILITY_DOCS,
  description: {
    component: [
      OVERLAY_HOST_RESPONSIBILITY_DOCS,
      "Provide dialog titles or aria-labelledby relationships, popover trigger labels, collapsible trigger text, and scrollable region labels from the host app.",
      "Keep long dialog, popover, collapsible, and scrollable content overflow-safe on mobile widths.",
    ].join(" "),
  },
};
