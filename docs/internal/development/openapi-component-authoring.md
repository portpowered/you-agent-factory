# OpenAPI Component Authoring

`api/openapi-main.yaml` is the authored OpenAPI entrypoint, but reusable
components must not be declared inline there. Put reusable component bodies in
files under `api/components/`, then reference those files from
`api/openapi-main.yaml` with a single-object `$ref` entry.

This rule applies to reusable component sections such as `schemas`,
`parameters`, `responses`, `examples`, `headers`, `requestBodies`, and
`securitySchemes` when they are present.

## Accepted

```yaml
components:
  schemas:
    SubmitWorkRequest:
      $ref: './components/schemas/api/SubmitWorkRequest.yaml'
  responses:
    BadRequest:
      $ref: './components/responses/BadRequest.yaml'
```

Each component key in `api/openapi-main.yaml` points at exactly one component
file. The component file owns the schema, response, parameter, or other
reusable body.

## Rejected

```yaml
components:
  schemas:
    SubmitWorkRequest:
      type: object
      properties:
        workTypeName:
          type: string
  responses:
    BadRequest:
      description: Invalid request
```

Inline reusable component bodies make the main spec harder to review and
increase merge conflict risk. A `$ref` entry with sibling fields is also
rejected; move those fields into the referenced component file instead.

## Verification

Run the canonical API smoke check before review:

```sh
make api-smoke
```

For a faster focused check while editing the authored entrypoint, run:

```sh
node scripts/run-quiet-api-command.js validate:main ./api/openapi-main.yaml
```

The component-decomposition lint runs before bundling or generation work and
reports the violating component section plus component name.
