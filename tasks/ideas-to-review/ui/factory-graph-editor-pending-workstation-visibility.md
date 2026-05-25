## Problem

The dashboard graph editor currently accepts a newly added workstation into the draft and enables `Save changes`, but the pending workstation node does not render in the live graph surface before save. On the rebased `main` surface, that means browser-driven route creation cannot target the newly added workstation even though the draft state already contains it.

## Why This Matters

This is a reusable architecture problem in the current graph-editor workflow, not a one-off test issue:

- browser-backed graph-editor coverage cannot create connections involving draft-added workstations before save
- contributors can misread the editor as ignoring a successful workstation addition
- current-activity graph rendering and draft-topology rendering are diverging in a way that hides valid draft state

## Suggested Direction

- make the current-activity graph editor render pending added workstation nodes from the draft topology, not only from the projected runtime layout
- keep the visible node IDs aligned with the draft connection controller so anchor clicks and pending-edge summaries operate on the same topology model
- add focused UI/runtime coverage that proves a newly added workstation becomes visible and can receive a route before the draft is saved
