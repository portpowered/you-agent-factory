# you-agent-factory

[![CI](https://github.com/portpowered/you-agent-factory/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/portpowered/you-agent-factory/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/portpowered/you-agent-factory?display_name=tag)](https://github.com/portpowered/you-agent-factory/releases)
[![Go Version](https://img.shields.io/badge/go-1.24-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE.md)

**you-agent-factory** gives a job to a team of AI helpers. The helpers can work at the same time, so you do not have to wait for one before starting another.

![The you-agent-factory dashboard](./docs/internal/resources/dashboard.png)

## What does it do?

Think of it as a small factory for AI work. You pick the steps. The factory sends each job to a helper.

It works with OpenAI Codex, Claude, and your own command-line helpers.

It can help you:

- run many helpers at once
- ask another helper to check the work
- try again if something needs fixing
- start jobs at a set time

## Installation

### Prerequisites

- A project folder
- The **[Codex CLI](https://developers.openai.com/codex/cli)**. Install it with:

  ```sh
  npm i -g @openai/codex
  ```

### Install the `you` CLI

**macOS / Linux:**

```sh
curl -fsSL https://github.com/portpowered/you-agent-factory/releases/latest/download/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://github.com/portpowered/you-agent-factory/releases/latest/download/install.ps1 | iex
```

## Quick start
#### Basic

```
you run --named "@you/goal" --default-worker-model-provider "codex"  "go update my readme for me to be more emoji like"
```

#### Complex
Open a terminal in the project you want help with:

1. Go to the project: `cd your-project-directory`
2. Start the factory: `you`
3. Open the link it shows. It is usually `http://localhost:7437/dashboard/ui`.
4. Give it a job. For example: “Write a report about my code in `TEST.md`.”

## Features


### Tools
1. graph orchestration: use `you` to run multiple agents in a graph, so that you can have agents hand off work from one point to another. 
2. dynamic javascript orchestration: use `you` to run javascript based custom workflows that chain agents together. 
3. cross harness interop: use `you` across a variety of agent harnesses including codex, cursor, claude, opencode, pi kiro, antigravity, and others.
4. website visualization and configuration management. use the `you` self hosted website to control a view of all your work in progress.
5. command line: use `you` cli to create and spawn independent factories of agents from the command line. manage factories from the command line. 


### Complex graph support

you agent factory supports a very complex comes with support for

1. interactive replay of events over time, see where things fail and what happened
2. parallel session support, run multiple factories in different folders at the same time
3. scripts and agents, use scripts when you want deterministic logic and agents for when you want flexibility. 
4. split and merge. you agent factories, lets work to merge and split to make really complex workflows happen. 
5. ticket based. you agent factory lets you create tickets in linear and github and poll them for work. 
6. crons. you agent factory lets you create crons to trigger work at bespoke timing. 
7. 

### Harness support

### packaged factories

You comes with some packaged factories that customers can use by default: 

1. `@you/goal` lets customers run a command forever, until the agent feels its complete. 
2. `@you/subagent` lets customers run against any harness with the same `you` CLI shell and have support for worktrees, skip permissions
3. `@you/meta` runs the same type of factory as the you-agent-factory. it takes in an idea, converts into a big plan, creates lots of separate agents, and merges them to completion in github. 
4. `@you/tts` runs a local tts model to convert some text into audio
5. `@you/loop` runs a session that repeats a command every X hours.
6. `@you/fusion` lets you have one agent do something, and then have another agent review the results
7. `@you/reviewer` lets you have one agent do something, and have another agent review the results until it completes.
8. `@you/ralph` converts your ask into a plan, and has the agent repeat until the plan is complete. 

## Make it your own

Choose where work goes next. For example: a writer works first, a reviewer checks it, then the writer fixes any problems.

Start with an [example factory](./examples/), or make your own with `you init`. You can give helpers rules in `AGENTS.md` and change the steps in `factory.json`.

## References

- [Make your own factory](./docs/reference/authoring-factories.md)
- [Command-line help](./docs/reference/README.md)
- [How it works](./docs/architecture/architecture.md)

## Comparison

Want to see how it compares with other tools? Read the [comparison](./docs/comparatives/comparing-systems.md).

## License

This repository is released under the [MIT License](./LICENSE.md).