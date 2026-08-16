import { Component, Fragment, type ReactNode } from "react";
import { Button } from "../../../components/ui/button";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";

interface CurrentActivityGraphErrorBoundaryProps {
  children: ReactNode;
  locale?: string;
}

interface CurrentActivityGraphErrorBoundaryState {
  error: Error | null;
  retryKey: number;
}

/**
 * Contains renderer failures to the graph surface. The surrounding card owns
 * the canonical document and remains mounted while this disposable surface
 * is retried.
 */
export class CurrentActivityGraphErrorBoundary extends Component<
  CurrentActivityGraphErrorBoundaryProps,
  CurrentActivityGraphErrorBoundaryState
> {
  public state: CurrentActivityGraphErrorBoundaryState = {
    error: null,
    retryKey: 0,
  };

  public static getDerivedStateFromError(
    error: Error,
  ): Partial<CurrentActivityGraphErrorBoundaryState> {
    return { error };
  }

  public render() {
    if (this.state.error !== null) {
      const messages = getWorkflowActivityShellMessages(this.props.locale);

      return (
        <section
          aria-label={messages.rendererErrorTitle}
          className="grid min-h-96 flex-1 place-items-center gap-4 rounded-3xl border border-af-danger-border bg-error-container p-6 text-center text-on-error-container"
          data-current-activity-graph-renderer-error="true"
        >
          <div className="grid gap-1">
            <h3 className="m-0 text-sm font-semibold">
              {messages.rendererErrorTitle}
            </h3>
            <p className="m-0 max-w-prose text-sm leading-6" role="alert">
              {messages.rendererErrorMessage}
            </p>
          </div>
          <Button
            autoFocus
            onClick={() => {
              this.setState((state) => ({
                error: null,
                retryKey: state.retryKey + 1,
              }));
            }}
            type="button"
          >
            {messages.rendererRetryLabel}
          </Button>
        </section>
      );
    }

    return <Fragment key={this.state.retryKey}>{this.props.children}</Fragment>;
  }
}
