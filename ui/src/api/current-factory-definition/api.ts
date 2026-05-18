import {
  getCurrentFactory,
  NamedFactoryAPIError,
  type GetCurrentFactoryOptions,
  type NamedFactoryAPIErrorCode,
} from "../named-factory";
import {
  FactoryDefinitionAPIError,
  normalizeFactoryDefinition,
  type CanonicalFactoryDefinition,
} from "../factory-definition";

export type { CanonicalFactoryDefinition } from "../factory-definition";

export type CurrentEditableFactoryDefinitionErrorCode =
  | NamedFactoryAPIErrorCode
  | "INVALID_FACTORY_DEFINITION";

export interface CurrentEditableFactoryDefinitionErrorDetails {
  cause?: unknown;
  code: CurrentEditableFactoryDefinitionErrorCode;
  responseBody?: unknown;
  status?: number;
  statusText?: string;
}

export class CurrentEditableFactoryDefinitionError extends Error {
  public readonly cause?: unknown;
  public readonly code: CurrentEditableFactoryDefinitionErrorCode;
  public readonly responseBody?: unknown;
  public readonly status?: number;
  public readonly statusText?: string;

  public constructor(
    message: string,
    details: CurrentEditableFactoryDefinitionErrorDetails,
  ) {
    super(message);
    this.name = "CurrentEditableFactoryDefinitionError";
    this.cause = details.cause;
    this.code = details.code;
    this.responseBody = details.responseBody;
    this.status = details.status;
    this.statusText = details.statusText;
  }
}

export async function getCurrentEditableFactoryDefinition(
  options: GetCurrentFactoryOptions = {},
): Promise<CanonicalFactoryDefinition> {
  let currentFactory: unknown;

  try {
    currentFactory = await getCurrentFactory(options);
  } catch (error) {
    if (error instanceof NamedFactoryAPIError) {
      throw new CurrentEditableFactoryDefinitionError(error.message, {
        cause: error,
        code: error.code,
        responseBody: error.responseBody,
        status: error.status,
        statusText: error.statusText,
      });
    }

    throw error;
  }

  try {
    return normalizeFactoryDefinition(currentFactory);
  } catch (error) {
    if (error instanceof FactoryDefinitionAPIError) {
      throw new CurrentEditableFactoryDefinitionError(
        `The current factory API returned a factory definition the dashboard cannot edit. ${error.message}`,
        {
          cause: error,
          code: "INVALID_FACTORY_DEFINITION",
          responseBody: currentFactory,
        },
      );
    }

    throw error;
  }
}
