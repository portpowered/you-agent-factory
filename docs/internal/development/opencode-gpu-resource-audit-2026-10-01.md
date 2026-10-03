# GPU Resource Audit — 2026-10-01

Source: `packages/packaged-factories/factories/dub-video/factory.yaml` (verified only).

## Capacity

The factory declares a single resource:

- `gpu` with `capacity: 1`

## Workstations requesting the GPU

Three of the four workstations request `gpu` with `capacity: 1`:

1. `transcribe-and-save` (worker `transcribe-video`)
2. `translate-and-validate` (worker `translate-video-segments`)
3. `synthesize-from-source-audio` (worker `synthesize-video-segments`)

Because the resource total is `1`, these guarded stages run exclusively — only one may hold the GPU at a time.

## Rendering

The fourth workstation, `render-subtitles-and-dub` (worker `render-dubbed-video`), has no `resources` block: rendering does not use the GPU resource.
