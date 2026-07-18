import { Component, type ErrorInfo, type ReactNode } from "react";

import type { FactoryVisualizerError } from "./factory-topology-replay-types";

interface FactoryVisualizerErrorBoundaryProps {
  children: ReactNode;
  fallback: (error: FactoryVisualizerError) => ReactNode;
  onError?: (error: FactoryVisualizerError) => void;
  resetKey: unknown;
}

interface FactoryVisualizerErrorBoundaryState {
  error?: FactoryVisualizerError;
}

export class FactoryVisualizerErrorBoundary extends Component<
  FactoryVisualizerErrorBoundaryProps,
  FactoryVisualizerErrorBoundaryState
> {
  state: FactoryVisualizerErrorBoundaryState = {};

  static getDerivedStateFromError(cause: unknown) {
    return { error: renderingError(cause) };
  }

  componentDidCatch(_cause: unknown, _info: ErrorInfo) {
    if (this.state.error) this.props.onError?.(this.state.error);
  }

  componentDidUpdate(previous: FactoryVisualizerErrorBoundaryProps) {
    if (this.state.error && previous.resetKey !== this.props.resetKey) {
      this.setState({ error: undefined });
    }
  }

  render() {
    return this.state.error
      ? this.props.fallback(this.state.error)
      : this.props.children;
  }
}

function renderingError(cause: unknown): FactoryVisualizerError {
  return {
    cause: { name: cause instanceof Error ? cause.name : "UnknownError" },
    kind: "rendering",
    message: "Factory topology rendering failed.",
    recoverable: true,
  };
}
