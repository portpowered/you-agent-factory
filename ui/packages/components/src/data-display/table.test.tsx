// @vitest-environment happy-dom

import { axe } from "jest-axe";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./table";

describe("Table primitives", () => {
  it("renders semantic table structure with accessible headers and cells", async () => {
    renderPackageComponent(
      <main>
        <Table aria-label="Package table">
          <TableCaption>Caption text</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead scope="col">Name</TableHead>
              <TableHead scope="col">Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell>Alpha</TableCell>
              <TableCell>Ready</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </main>,
    );

    expect(
      screen.getByRole("table", { name: "Package table" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Caption text")).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Name" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Alpha" })).toBeInTheDocument();
    expect(await axe(document.body)).toHaveNoViolations();
  });

  it("applies compact dense spacing to headers and cells", () => {
    renderPackageComponent(
      <Table aria-label="Dense table" size="dense">
        <TableHeader>
          <TableRow>
            <TableHead scope="col">Name</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>Alpha</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    const scroller = screen.getByRole("table", {
      name: "Dense table",
    }).parentElement;
    expect(scroller).toHaveAttribute("data-size", "dense");
    expect(
      screen.getByRole("columnheader", { name: "Name" }).className,
    ).toContain("group-data-[size=dense]/table:h-8");
    expect(screen.getByRole("cell", { name: "Alpha" }).className).toContain(
      "group-data-[size=dense]/table:py-2",
    );
  });

  it("keeps horizontal scrolling inside the table container for narrow layouts", () => {
    renderPackageComponent(
      <Table
        aria-label="Scrollable table"
        containerClassName="min-w-0 overscroll-x-contain"
        containerProps={{ "data-testid": "table-scroller" } as never}
      >
        <TableHeader>
          <TableRow>
            <TableHead scope="col">Name</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>Alpha</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    const scroller = screen.getByTestId("table-scroller");
    expect(scroller.className).toContain("overflow-x-auto");
    expect(scroller.className).toContain("overscroll-x-contain");
    expect(scroller.className).toContain("min-w-0");
  });
});
