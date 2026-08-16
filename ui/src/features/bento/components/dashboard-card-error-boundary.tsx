import { Heading, Text } from "@you-agent-factory/components/primitives";
import {
  Component,
  type ErrorInfo,
  Fragment,
  type ReactNode,
  useId,
} from "react";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { getAgentBentoMessages } from "../messages/agent-bento";

export interface DashboardCardErrorBoundaryProps {
  children: ReactNode;
  locale?: string;
}

interface DashboardCardErrorBoundaryState {
  failed: boolean;
  retryRevision: number;
}

export class DashboardCardErrorBoundary extends Component<
  DashboardCardErrorBoundaryProps,
  DashboardCardErrorBoundaryState
> {
  public state: DashboardCardErrorBoundaryState = {
    failed: false,
    retryRevision: 0,
  };

  public static getDerivedStateFromError(): Pick<
    DashboardCardErrorBoundaryState,
    "failed"
  > {
    return { failed: true };
  }

  public componentDidCatch(_error: Error, _info: ErrorInfo): void {
    // Keep render diagnostics out of the customer-facing card recovery UI.
  }

  public render() {
    if (this.state.failed) {
      return (
        <DashboardCardErrorFallback
          locale={this.props.locale}
          onRetry={this.retry}
        />
      );
    }

    return (
      <Fragment key={this.state.retryRevision}>{this.props.children}</Fragment>
    );
  }

  private retry = () => {
    this.setState(({ retryRevision }) => ({
      failed: false,
      retryRevision: retryRevision + 1,
    }));
  };
}

interface DashboardCardErrorFallbackProps {
  locale?: string;
  onRetry: () => void;
}

function DashboardCardErrorFallback({
  locale,
  onRetry,
}: DashboardCardErrorFallbackProps) {
  const messages = getAgentBentoMessages(locale);
  const descriptionID = useId();

  return (
    <DashboardPanelShell
      aria-describedby={descriptionID}
      aria-label={messages.cardErrorTitle}
      as="article"
      className="flex h-full min-w-0 flex-col justify-between gap-4 p-4"
      shellKind="grid-card"
    >
      <div className="grid gap-2">
        <Heading as="h3">{messages.cardErrorTitle}</Heading>
        <Text as="p" id={descriptionID} role="alert">
          {messages.cardErrorDescription}
        </Text>
      </div>
      <DashboardActionButton onClick={onRetry} type="button">
        {messages.retryCard}
      </DashboardActionButton>
    </DashboardPanelShell>
  );
}
