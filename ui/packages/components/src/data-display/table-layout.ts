/** Wrap long cell content without forcing page-level horizontal overflow. */
export const tableCellWrapClassName = "min-w-0 [overflow-wrap:anywhere]";

/** Truncate overflowing cell content with ellipsis. */
export const tableCellTruncateClassName =
  "block min-w-0 max-w-full truncate whitespace-nowrap";

/** Minimum table width for wide datasets; pair with a narrow scroll container. */
export const tableMinWidthWideClassName = "min-w-2xl";

/** Scroll container classes for narrow viewport table containment. */
export const tableNarrowContainerClassName = "min-w-0 overscroll-x-contain";
