// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen, within } from "../testing/render";
import { WidgetFrame } from "./widget-frame";
import {
  WidgetErrorState,
  WidgetLoadingState,
  WidgetSuccessState,
} from "./widget-frame-states";

describe("widget frame host content", () => {
  it("renders arbitrary host slot content without requiring a dashboard content shape", () => {
    renderPackageComponent(
      <WidgetFrame title="Custom host widget">
        <section data-testid="host-slot">
          <ul>
            <li>First arbitrary row</li>
            <li>Second arbitrary row</li>
          </ul>
          <table>
            <tbody>
              <tr>
                <td>Cell value</td>
              </tr>
            </tbody>
          </table>
        </section>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Custom host widget" });
    const slot = within(card).getByTestId("host-slot");

    expect(within(slot).getByText("First arbitrary row")).toBeTruthy();
    expect(within(slot).getByText("Cell value")).toBeTruthy();
  });

  it("renders host-provided loading, error, and success panels inside the frame", () => {
    renderPackageComponent(
      <WidgetFrame title="Stateful widget">
        <WidgetLoadingState>Loading host content...</WidgetLoadingState>
        <WidgetErrorState>Host error copy</WidgetErrorState>
        <WidgetSuccessState>Host success copy</WidgetSuccessState>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Stateful widget" });

    expect(within(card).getByText("Loading host content...")).toBeTruthy();
    expect(within(card).getByRole("alert").textContent).toContain(
      "Host error copy",
    );
    expect(within(card).getByText("Host success copy")).toBeTruthy();
  });
});
