export function defineDataError(name, code, fields) {
  const DataError = function (...args) {
    return {
      name,
      message: fields.message(...args),
      code,
      ...fields.details(...args),
    };
  };
  Object.defineProperty(DataError, "name", { value: name });
  Object.defineProperty(DataError, Symbol.hasInstance, {
    value(candidate) {
      return candidate !== null
        && typeof candidate === "object"
        && Object.getPrototypeOf(candidate) === Object.prototype
        && candidate.name === name
        && candidate.code === code;
    },
  });
  return DataError;
}
