// Package internal composes the Factory Runtime composed root, hosted-instance
// lifecycle, runtime factory construction, assembly, and runtime sidecars.
// Parent-private orchestration, instance_host, and dispatch_planning subservices
// remain under internal/services/*. Peers should construct through
// factory_runtime/wire; baseline deletion of transitional service/ is owned by
// DEL-RUN.
package internal
