import {
  type Dispatch,
  type SetStateAction,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import {
  invokeSessionFactory,
  SessionFactoryInvocationError,
  type SessionFactoryInvocationResponse,
} from "../../../api/session-factory";
import { useDashboardBentoStore } from "../../bento/public";
import { useCurrentFactoryDefinition } from "../../current-factory-definition/public";
import {
  collectInvocationFieldErrors,
  extractInvocationFieldError,
  projectInvocationForm,
  serializeInvocationArgs,
} from "../lib/factory-invocation-form";
import type { SubmitWorkMessages } from "../messages/submit-work";

export interface FactoryInvocationStatusState {
  kind: "error" | "idle" | "submitting" | "success" | "validation-error";
  message?: string;
  response?: SessionFactoryInvocationResponse;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the hook keeps invocation state, submission, and feedback wiring in one public controller boundary.
export function useFactoryInvocationWidget(
  sessionID: string | null | undefined,
  messages: SubmitWorkMessages,
) {
  const currentFactory = useCurrentFactoryDefinition();
  const incrementRefreshToken = useDashboardBentoStore(
    (state) => state.incrementRefreshToken,
  );
  const signature = currentFactory.data?.invocationSignature;
  const projection = projectInvocationForm(
    signature,
    currentFactory.data?.examples,
  );
  const signatureKey = useMemo(
    () => (signature ? JSON.stringify(signature) : null),
    [signature],
  );
  const [fieldValues, setFieldValues] = useState<Record<string, string[]>>({});
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [status, setStatus] = useState<FactoryInvocationStatusState>({
    kind: "idle",
  });
  const [isSubmitting, setIsSubmitting] = useState(false);
  useFactoryInvocationResetOnSignatureChange(
    sessionID,
    signatureKey,
    setFieldValues,
    setFieldErrors,
    setStatus,
  );

  const submit = async () => {
    const nextFieldErrors = collectInvocationFieldErrors(
      projection.fields,
      fieldValues,
      {
        repeatedItemRequired:
          messages.invocation.validationMessages.repeatedItemRequired,
        requiredFieldMessage: (label) =>
          messages.invocation.validationMessages.requiredField(label),
      },
    );
    if (Object.keys(nextFieldErrors).length > 0) {
      setFieldErrors(nextFieldErrors);
      setStatus({
        kind: "validation-error",
        message: messages.invocation.statusMessages.validationFailed,
      });
      return;
    }

    setFieldErrors({});
    setIsSubmitting(true);
    setStatus({
      kind: "submitting",
      message: messages.invocation.statusMessages.submitting,
    });

    try {
      const args = serializeInvocationArgs(projection.fields, fieldValues);
      const response = await invokeSessionFactory(
        {
          args,
        },
        { sessionID },
      );

      incrementRefreshToken();
      if (response.status === "COMPLETED") {
        setStatus({
          kind: "success",
          message: messages.invocation.statusMessages.success(response.traceId),
          response,
        });
      } else {
        setStatus({
          kind: "error",
          message:
            response.message ??
            messages.invocation.statusMessages.runtimeFailed(response.status),
          response,
        });
      }
    } catch (error) {
      if (error instanceof SessionFactoryInvocationError) {
        const invocationFieldError = extractInvocationFieldError(
          projection.fields,
          error.message,
        );
        if (invocationFieldError) {
          setFieldErrors((current) => ({
            ...current,
            [invocationFieldError.fieldName]: invocationFieldError.message,
          }));
        }
        setStatus({
          kind:
            invocationFieldError ||
            error.code.startsWith("INVOCATION_ARGUMENT_")
              ? "validation-error"
              : "error",
          message: error.message,
        });
      } else {
        setStatus({
          kind: "error",
          message: messages.invocation.statusMessages.errorFallback,
        });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return {
    currentFactory,
    fieldErrors,
    fieldValues,
    isSubmitting,
    projection,
    setBooleanValue: (name: string, value: "false" | "true" | undefined) => {
      setInvocationArgumentValues(
        name,
        value === undefined ? [] : [value],
        setFieldValues,
        setFieldErrors,
      );
    },
    setFieldValue: (name: string, value: string) => {
      setInvocationArgumentValues(
        name,
        [value],
        setFieldValues,
        setFieldErrors,
      );
    },
    setRepeatedFieldValue: (name: string, values: string[]) => {
      setInvocationArgumentValues(name, values, setFieldValues, setFieldErrors);
    },
    status,
    submit,
  };
}

function useFactoryInvocationResetOnSignatureChange(
  sessionID: string | null | undefined,
  signatureKey: string | null,
  setFieldValues: Dispatch<SetStateAction<Record<string, string[]>>>,
  setFieldErrors: Dispatch<SetStateAction<Record<string, string>>>,
  setStatus: Dispatch<SetStateAction<FactoryInvocationStatusState>>,
) {
  const resetContextRef = useRef<{
    sessionID: string | null | undefined;
    signatureKey: string | null;
  } | null>(null);

  useEffect(() => {
    const previousContext = resetContextRef.current;
    resetContextRef.current = { sessionID, signatureKey };
    if (previousContext == null) {
      return;
    }
    if (
      previousContext.sessionID === sessionID &&
      (signatureKey === null || previousContext.signatureKey === signatureKey)
    ) {
      return;
    }
    setFieldValues({});
    setFieldErrors({});
    setStatus({ kind: "idle" });
  }, [sessionID, setFieldErrors, setFieldValues, setStatus, signatureKey]);
}

function setInvocationArgumentValues(
  name: string,
  values: string[],
  setFieldValues: Dispatch<SetStateAction<Record<string, string[]>>>,
  setFieldErrors: Dispatch<SetStateAction<Record<string, string>>>,
) {
  setFieldValues((current) => ({
    ...current,
    [name]: values,
  }));
  setFieldErrors((current) => {
    const next = { ...current };
    delete next[name];
    return next;
  });
}
