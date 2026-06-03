import "../../../styles.css";

import { COLOR_PALETTE_OPTIONS } from "../../../theme";
import { DashboardHeader } from "./dashboard-header";

export default {
  title: "you-agent-factory/Dashboard/Color Palette Selector",
  component: DashboardHeader,
  tags: ["test"],
};

export const PaletteOptions = {
  render: () => (
    <div style={{ margin: "0 auto", maxWidth: "1280px", width: "100%" }}>
      <p className="mb-3 text-on-surface-variant">
        Open the palette dropdown beside the language control to preview{" "}
        {COLOR_PALETTE_OPTIONS.length} predefined palettes.
      </p>
      <DashboardHeader />
    </div>
  ),
};
