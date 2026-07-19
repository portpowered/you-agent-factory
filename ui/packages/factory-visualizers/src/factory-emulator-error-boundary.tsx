import { Button, Text } from "@you-agent-factory/components";
import { Component, type ErrorInfo, type ReactNode } from "react";

import {
  type FactoryVisualizerError,
  normalizeFactoryVisualizerError,
} from "./visualizer-error";

/** A host-provided, safe-to-display local failure and optional recovery action. */
export interface FactoryEmulatorFailure {
  message: string;
  recoveryAction?: {
    label: string;
    onRecover: () => void;
  };
}

interface FactoryEmulatorErrorBoundaryProps {
  children: ReactNode;
  failure?: FactoryEmulatorFailure;
  onError?: (error: FactoryVisualizerError) => void;
  regionLabel: string;
}

interface FactoryEmulatorErrorBoundaryState {
  error?: FactoryVisualizerError;
}

export class FactoryEmulatorErrorBoundary extends Component<
  FactoryEmulatorErrorBoundaryProps,
  FactoryEmulatorErrorBoundaryState
> {
  state: FactoryEmulatorErrorBoundaryState = {};

  static getDerivedStateFromError(): FactoryEmulatorErrorBoundaryState {
    return { error: normalizeFactoryVisualizerError(undefined, "render") };
  }

  componentDidCatch(error: unknown, _errorInfo: ErrorInfo) {
    const diagnostic = normalizeFactoryVisualizerError(error, "render");
    this.setState({ error: diagnostic });
    this.props.onError?.(diagnostic);
  }

  render() {
    if (!this.state.error && !this.props.failure) return this.props.children;
    const failure = this.props.failure;
    return (
      <section
        aria-label={this.props.regionLabel}
        className="factory-emulator-failure"
      >
        <div className="factory-emulator-failure__content" role="alert">
          <Text as="p">
            {failure?.message ?? "The Factory emulator could not be shown."}
          </Text>
          {failure?.recoveryAction ? (
            <Button onClick={failure.recoveryAction.onRecover} type="button">
              {failure.recoveryAction.label}
            </Button>
          ) : null}
        </div>
      </section>
    );
  }
}
