import { Button, Text } from "@you-agent-factory/components";
import {
  Component,
  type ErrorInfo,
  type ReactNode,
  useEffect,
  useRef,
} from "react";

import type {
  FactoryTopologyReplayMessages,
  FactoryTopologyReplayProps,
} from "./factory-topology-replay";
import {
  type FactoryTopologyReplayError,
  type FactoryVisualizerError,
  type FactoryVisualizerErrorKind,
  factoryVisualizerErrorKey,
  normalizeFactoryVisualizerError,
  toFactoryVisualizerError,
} from "./visualizer-error";

export function FactoryTopologyStateRegion({
  messages,
  onRetry,
  state,
}: {
  messages: FactoryTopologyReplayMessages;
  onRetry?: () => void;
  state: "empty" | "failed" | "loading";
}) {
  return (
    <section
      aria-busy={state === "loading" ? "true" : undefined}
      aria-label={messages.regionLabel}
      className="factory-topology-replay factory-topology-replay--state"
    >
      <FactoryTopologyStatePresentation
        messages={messages}
        onRetry={onRetry}
        state={state}
      />
    </section>
  );
}

function FactoryTopologyStatePresentation({
  messages,
  onRetry,
  state,
}: {
  messages: FactoryTopologyReplayMessages;
  onRetry?: () => void;
  state: "empty" | "failed" | "loading";
}) {
  return (
    <div
      className="factory-topology-replay__state"
      role={state === "failed" ? "alert" : "status"}
    >
      <Text as="p">{messages[state]}</Text>
      {state === "failed" && onRetry ? (
        <Button onClick={onRetry} type="button">
          {messages.retry}
        </Button>
      ) : null}
    </div>
  );
}

interface FactoryTopologyErrorBoundaryProps {
  children: ReactNode;
  errorKind: Extract<FactoryVisualizerErrorKind, "react-flow" | "render">;
  messages: FactoryTopologyReplayMessages;
  onError?: (error: FactoryVisualizerError) => void;
  onRetry?: () => void;
  resetKeys: readonly unknown[];
  withinRegion?: boolean;
}

interface FactoryTopologyErrorBoundaryState {
  error?: FactoryVisualizerError;
}

export class FactoryTopologyErrorBoundary extends Component<
  FactoryTopologyErrorBoundaryProps,
  FactoryTopologyErrorBoundaryState
> {
  state: FactoryTopologyErrorBoundaryState = {};
  private readonly reportedErrors = new Set<string>();

  static getDerivedStateFromError(): FactoryTopologyErrorBoundaryState {
    return { error: toFactoryVisualizerError("render") };
  }

  componentDidCatch(error: unknown, _errorInfo: ErrorInfo) {
    const diagnostic = normalizeFactoryVisualizerError(
      error,
      this.props.errorKind,
    );
    this.setState({ error: diagnostic });
    this.report(diagnostic);
  }

  componentDidUpdate(previousProps: FactoryTopologyErrorBoundaryProps) {
    if (
      this.state.error &&
      resetKeysChanged(previousProps.resetKeys, this.props.resetKeys)
    ) {
      this.setState({ error: undefined });
    }
  }

  render() {
    if (!this.state.error) return this.props.children;
    const presentation = (
      <FactoryTopologyStatePresentation
        messages={this.props.messages}
        onRetry={this.props.onRetry}
        state="failed"
      />
    );
    return this.props.withinRegion ? (
      presentation
    ) : (
      <section
        aria-label={this.props.messages.regionLabel}
        className="factory-topology-replay factory-topology-replay--state"
      >
        {presentation}
      </section>
    );
  }

  private report(error: FactoryVisualizerError) {
    const key = factoryVisualizerErrorKey(error);
    if (this.reportedErrors.has(key)) return;
    this.reportedErrors.add(key);
    this.props.onError?.(error);
  }
}

export function useDistinctTopologyErrorReport(
  error: FactoryTopologyReplayError | undefined,
  onError: FactoryTopologyReplayProps["onError"],
) {
  const reportedErrors = useRef(new Set<string>());
  useEffect(() => {
    if (!error) return;
    const key =
      error.kind === "layout-validation"
        ? error.issues
            .map((issue) => `${issue.category}:${issue.code}:${issue.path.join(".")}`)
            .join("|")
        : factoryVisualizerErrorKey(error);
    if (reportedErrors.current.has(key)) return;
    reportedErrors.current.add(key);
    onError?.(error);
  }, [error, onError]);
}

function resetKeysChanged(
  previous: readonly unknown[],
  current: readonly unknown[],
): boolean {
  return (
    previous.length !== current.length ||
    previous.some((value, index) => value !== current[index])
  );
}
