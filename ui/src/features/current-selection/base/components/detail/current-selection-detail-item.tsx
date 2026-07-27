import type { ReactNode } from "react";

import { Code } from "@you-agent-factory/components/primitives";
import { cn } from "../../../../../lib/cn";

export interface CurrentSelectionDetailItemProps {
  children?: ReactNode;
  code?: boolean;
  label: ReactNode;
  rawValue?: string;
  value?: ReactNode;
}

export function CurrentSelectionDetailItem({
  children,
  code = false,
  label,
  rawValue,
  value,
}: CurrentSelectionDetailItemProps) {
  const content = children ?? value;

  if (content === undefined || content === null || content === "") {
    return null;
  }

  return (
    <div>
      <dt>{label}</dt>
      <CurrentSelectionDetailValue>
        {code ? (
          <CurrentSelectionDetailCode>{content}</CurrentSelectionDetailCode>
        ) : rawValue ? (
          <span title={rawValue}>{content}</span>
        ) : (
          content
        )}
      </CurrentSelectionDetailValue>
    </div>
  );
}

export function CurrentSelectionDetailValue({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <dd className={cn("min-w-0 [overflow-wrap:anywhere]", className)}>
      {children}
    </dd>
  );
}

export function CurrentSelectionDetailCode({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <Code className={cn("[overflow-wrap:anywhere]", className)}>
      {children}
    </Code>
  );
}
