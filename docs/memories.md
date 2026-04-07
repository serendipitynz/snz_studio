# Memory Specification

## Purpose

Memories store durable project context that should remain useful across multiple chats.

They are not intended to replace documents or raw chat history.

In practice:

- documents are the source material
- chat summaries capture the flow of each chat
- memories keep compressed, reusable facts and rules

## Memory Kinds

### Procedural

How the assistant should work.

Examples:

- writing style preferences
- translation rules
- output formatting rules
- restrictions such as forbidden terms

### Semantic

Stable project facts.

Examples:

- worldbuilding facts
- character facts
- terminology decisions
- fixed technical or structural assumptions

### Episodic

Past decisions or confirmed events.

Examples:

- an important story event already established
- a previous editorial decision
- a confirmed project milestone

## Data Model

Each memory stores:

- `id`
- `project_id`
- `kind`
- `title`
- `content`
- `source_chat_id`
- `source`
- `locked`
- `created_at`
- `updated_at`

### Source

`source` indicates where the memory came from:

- `manual`
- `chat`
- `organized`

### Locked

`locked` means the memory is protected from automatic organizer rewrite or removal.

It does not prevent manual user actions.
Users can still:

- unlock it
- lock it again
- delete it manually

## Creation Paths

### Manual creation

Users can create memories from the project detail screen.

Current behavior:

- the user selects `kind`
- the user enters `content`
- the title is generated automatically from the content
- manual memories default to `locked = true`
- `source = manual`

### Automatic extraction from user chat input

After each user message, the backend may create memories from the user input.

Important constraints:

- only user messages are analyzed
- assistant messages are not converted into memories
- questions are ignored
- long sentences are ignored
- only messages matching durable heuristics are considered

This extraction is intentionally conservative.

Examples of heuristic matches:

- procedural cues: "please use", "日本語で", "簡潔に"
- semantic cues: "this project uses", "このアプリは", "前提です"
- episodic cues: "we decided", "前回", "変更した"

Automatically extracted memories are created with:

- `source = chat`
- `locked = false`

## Retrieval and Usage in Chat

Memories are not all injected into every response.

Current behavior:

1. up to 4 recent `procedural` memories are always included as persistent working instructions
2. additional relevant memories are retrieved by FTS and optional embedding-based reranking
3. only the selected memories are attached to the assistant turn as references

This means:

- procedural memories strongly influence how the assistant answers
- semantic and episodic memories are mostly pulled in on demand

## Organizer

The memory organizer is a manual tool.
It does not run on every chat turn.

It analyzes:

- current memories
- recent chat summaries

It may propose:

- `create`
- `update`
- `remove`

Rules:

- locked memories are not updated or removed by the organizer
- manual review happens in the UI before applying changes

## What Should Become Memory

Good candidates:

- durable instructions
- stable facts
- confirmed project events
- short statements reused across chats

Bad candidates:

- long source material
- full story text
- temporary brainstorming
- casual conversation fragments
- uncertain drafts

## Recommended Operating Rule

Use memories for short, reusable context.
Use documents for detailed material.
Use chat summaries for conversation progress.
