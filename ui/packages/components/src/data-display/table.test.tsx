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
    expect(screen.getByRole("columnheader", { name: "Name" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Alpha" })).toBeInTheDocument();
    expect(await axe(document.body)).toHaveNoViolations();
  });
});
