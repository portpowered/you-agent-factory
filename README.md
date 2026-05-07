# Infinite You

[![CI](https://github.com/portpowered/infinite-you/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/portpowered/infinite-you/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/portpowered/infinite-you?display_name=tag)](https://github.com/portpowered/infinite-you/releases)
[![Go Version](https://img.shields.io/badge/go-1.24-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE.md)

Infinite You is an AI agent factory. It orchestrates AI agents for you so you can do more work without doing everything manually.

## Why?

Leverage. 

With __Infinite You__, you codify your process into a workflow with different AGENTs.md and run them as wrappers around OpenAI codex.

For example: 
- dispatch 10 agents to run independently in separate work trees
- have one agent loop through a series of tasks, and then have a reviewer review the output and retrigger the loop if it failed
- tell the agents a series of plans, and run them in dependency order
- have a cron setup to autonomously look at git tasks or whatever and submit tasks that go through a write/review cycle loop

## Install


1. install [codex](https://developers.openai.com/codex/cli) `npm i -g @openai/codex`
2. install on macOS/Linux: `curl -fsSL https://github.com/portpowered/infinite-you/releases/latest/download/install.sh | sh`
3. install on Windows PowerShell: `irm https://github.com/portpowered/infinite-you/releases/latest/download/install.ps1 | iex`
4. go `cd your-project-directory`
5. run `infinite-you`
6. submit a work task on the website interface, like "go write a report on my codebase at TEST.md", 
7. wait till complete
8. finished


### claude variant
```
infinite-you init --executor claude --dir my-factory
infinite-you docs workstation
```


## Example
Here's an example of the factory for infinite-you dispatching roughly 5-10 agents. 

![](docs/internal/resources/dashboard.gif)


## How It Works


The default no-argument starter flow looks like below: you give it a task, it spawns a basic agent CLI run and does stuff. 
```mermaid
flowchart LR
   classDef place fill:#000,stroke:#333,color:#fff,stroke-width:2px
   classDef transition fill:#333,stroke:#333,color:#fff,rx:0,ry:0

   P0((task:init)):::place
   P1((task:complete)):::place
   P2((task:failed)):::place

   T0[process]:::transition

   P0 --> T0
   T0 --> P1
   T0 -.->|on failure| P2

```

## Customization 

See [authoring-workflows](./docs/reference/authoring-workflows.md) for the full configuration guide.
Infinite you lets you customize your flow however you want. 

The overall system of how __infinite you__ works is relatively simple. 
1. You have work. 
2. Work goes to workstations where the work gets worked on by workers (agents, or just shell scripts)
3. When the workstations complete the, work is converted to other work.  
4. __Infinite you__ stops when no work remains.


## Shipped example factories

Drag the images from the examples/factories directory into the web interface's flow graph, and it'll load the factory for you. 

<table>
  <tr>
    <td align="center">
      <strong>Doc reviewer</strong><br />
      Write and review workflow.<br />
      <img src="examples/factories/doc-reviewer.png" alt="Doc reviewer factory" width="200" />
    </td>
    <td align="center">
      <strong>Infinite you</strong><br />
      Meta factory that runs the factory.<br />
      <img src="examples/factories/infinite-you.png" alt="Infinite you factory" width="200" />
    </td>
    <td align="center">
      <strong>Ralph</strong><br />
      Iterative plan, code, and review loop.<br />
      <img src="examples/factories/ralph.png" alt="Ralph factory" width="200" />
    </td>
  </tr>
  <tr>
    <td align="center">
      <strong>Timer</strong><br />
      Cron-based factory trigger.<br />
      <img src="examples/factories/timer.png" alt="Timer factory" width="200" />
    </td>
    <td align="center">
      <strong>Worktree</strong><br />
      Spawns work in a git worktree.<br />
      <img src="examples/factories/worktree.png" alt="Worktree factory" width="200" />
    </td>
    <td align="center">
      <strong>Writer reviewer</strong><br />
      Iterative loop for writing docs.<br />
      <img src="examples/factories/writer-reviewer.png" alt="Writer reviewer factory" width="200" />
    </td>
  </tr>
</table>

### References

- [Analysis on current projects](docs/comparatives/comparing-systems.md)
