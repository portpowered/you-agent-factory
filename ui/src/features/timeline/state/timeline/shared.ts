export function uniqueSorted(
  values: Array<string | null | undefined>,
): string[] {
  return [
    ...new Set(
      values.filter(
        (value): value is string =>
          typeof value === "string" && value.length > 0,
      ),
    ),
  ].sort();
}
