
This project is infinite-you, an AI agent factory intended to schedule and orchestrate AI. 
The goal of the project generally is to rapidly allow customers to deploy lots of AI concurrently so 
as to maximize their overall productivity. 

It borrows heavily from the general best practices for manufacturing and engineering for its structures.

## technologies

The main technologies that are used are: 
1. golang backend server + CLI
2. website via react
3. api defined via openAPI

## repo structure

docs/ -> general documentation about the package, best practices for users, etc
docs/architecture/ -> architecture notes and data model writeups
docs/development/ -> active engineering notes, inventories, audits, and implementation closeout docs
docs/guides/ -> task-oriented user and developer guides
docs/processes/ -> process-specific reference docs and relevant-file maps
docs/reference/ -> reference material for config, resources, workers, and templates
docs/standards/ -> coding and workflow standards that should guide changes

pkg/ -> golang codebase for backend server, CLI
pkg/api/ -> API-facing server code, generated contracts, and API test data
pkg/apisurface/ -> public request/response surface and boundary shaping
pkg/cli/ -> CLI flows for init, run, submit, config, dashboard, and factory commands
pkg/config/ -> configuration parsing, runtime config, projections, and scheduler/state helpers
pkg/factory/ -> factory execution context, engine, runtime, and workstation config plumbing
pkg/interfaces/ -> shared interface definitions across subsystems
pkg/internal/ -> internal-only backend implementation details such as submission handling
pkg/listeners/ -> listener implementations and event ingestion hooks
pkg/service/ -> service-layer orchestration and backend coordination
pkg/workers/ -> worker runtime behavior and related test data

ui/ -> website codebase
ui/src/ -> React app source for API access, components, features, hooks, state, testing, and shared types
ui/scripts/ -> frontend build and tooling scripts
ui/integration/ -> integration-oriented frontend test assets
ui/.storybook/ -> Storybook configuration for UI component development

api/ -> openapi schema defining backend server API, used to generate code on website/backend
tests -> general tests for the backend as well as stress, and integration tests
factory/ -> automated code processes that bootstraps infinite-you with infinite-you
cmd/ -> golang entrypoints for cli and server and whatnot
examples/ -> example factory directories of infinite-you


## standards
please read the appropriate standards when performing the appropriate operation
1. docs/standards/code/code-review-standards.md for when reviewing changes
2. docs/standards/code/general-backend-standards.md for when modifying the backend
3. docs/standards/code/general-website-standards.md for when modifying the frontend
