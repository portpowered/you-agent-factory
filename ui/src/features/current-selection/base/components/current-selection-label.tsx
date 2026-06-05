import { forwardRef, type HTMLAttributes } from "react";

import { DashboardLabel } from "../../../../components/ui";

export interface CurrentSelectionLabelProps
  extends HTMLAttributes<HTMLSpanElement> {}

export const CurrentSelectionLabel = forwardRef<
  HTMLSpanElement,
  CurrentSelectionLabelProps
>(function CurrentSelectionLabel(props, ref) {
  return <DashboardLabel ref={ref} {...props} />;
});
