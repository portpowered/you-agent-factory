import { forwardRef, type HTMLAttributes } from "react";

import { Label } from "../../../../../components/ui";

export interface CurrentSelectionLabelProps
  extends HTMLAttributes<HTMLSpanElement> {}

export const CurrentSelectionLabel = forwardRef<
  HTMLSpanElement,
  CurrentSelectionLabelProps
>(function CurrentSelectionLabel(props, ref) {
  return <Label ref={ref} {...props} />;
});
