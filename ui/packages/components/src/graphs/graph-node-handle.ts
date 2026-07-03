export type GraphNodeHandleTone =
  | "assignment"
  | "continue"
  | "default"
  | "failure"
  | "input"
  | "output"
  | "rejection"
  | "resource"
  | "worker";

export interface GraphNodeHandle {
  buttonAriaLabel?: string;
  buttonDisabled?: boolean;
  buttonPressed?: boolean;
  buttonTitle?: string;
  connectable?: boolean;
  hidden?: boolean;
  id: string;
  label: string;
  onButtonClick?: () => void;
  side: "left" | "right";
  tone?: GraphNodeHandleTone;
  type: "source" | "target";
  validationError?: boolean;
  validationMessage?: string;
  variant?: "default" | "error" | "muted" | "selected" | "valid-target";
}
