// @vitest-environment node

import {
  COMPONENTS_CATEGORY,
  DataTable,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  tableCellWrapClassName,
  tableNarrowContainerClassName,
} from "@you-agent-factory/components/data-display";
import { describe, expect, it } from "vitest";

describe("data-display exports", () => {
  it("exposes table primitives and DataTable from the data-display entrypoint", () => {
    expect(COMPONENTS_CATEGORY).toBe("data-display");
    expect(Table).toBeTruthy();
    expect(TableHeader).toBeTruthy();
    expect(TableBody).toBeTruthy();
    expect(TableRow).toBeTruthy();
    expect(TableHead).toBeTruthy();
    expect(TableCell).toBeTruthy();
    expect(TableCaption).toBeTruthy();
    expect(typeof DataTable).toBe("function");
    expect(tableCellWrapClassName).toContain("overflow-wrap");
    expect(tableNarrowContainerClassName).toContain("overscroll-x-contain");
  });
});
