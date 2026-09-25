# AGENTS.md

## Goal
Build a minimal local single-user project workspace for a local LLM, with a UX loosely similar to ChatGPT / Claude Projects.

## Priorities
1. Speed of implementation
2. Simplicity
3. Readability
4. Local-only operation
5. Easy future extension

## Core product shape
The app should support:
- projects
- documents
- chats
- memories

Each chat belongs to a project.
Each project can have multiple documents and memories.
Chats should be able to use project documents and memories as context.

A chat is one of two kinds:

- single assistant: the user and one assistant model
- multi-agent conversation: two or more participants speaking in turn

A multi-agent conversation is not a separate product area beside projects / documents / chats /
memories; it is a kind of chat. It is made of:

- participants: display name + role prompt + endpoint + model, kept in roster order
- a turn rule that decides who speaks next (cycle the roster, nominate each speaker, or alternate a
  chosen facilitator with the rest of the roster)
- a scene shared by every participant (topic, setting, world)
- presets that fill participants / turn rule / scene in one step

One request runs one turn; the user watches, advances turns, and can speak into the conversation
at any point.

## Required UX
The UX should feel like a lightweight local version of ChatGPT Projects:
- each project has shared context
- multiple chats can exist under one project
- project documents can be referenced during chat
- previous chat history should be compressed via summaries
- persistent memories should be stored separately from raw messages
- the UI should show what references were used for an answer
- a multi-agent conversation should be watchable turn by turn, with its roster and scene editable while it runs

## Constraints
- Single user only
- Local machine only
- No auth
- No multi-user support
- No PDF support
- No OCR
- No vector database
- No Electron
- No external infrastructure
- No heavy real-time architecture
- No over-engineering
- No tailwind
- No `window.confirm` / `window.alert` / `window.prompt` — the Wails WebView never shows
  them and resolves `confirm` to `false`; use the in-app dialog (`components/ConfirmDialog`)

## Tech preferences
- Desktop shell: Wails v2 (Go core + OS-native WebView)
- Frontend: React + Vite + TypeScript (No tailwind)
- Backend: Go local API server (`net/http` over loopback, SSE preserved)
- Database: SQLite (`modernc.org/sqlite`, pure Go)
- File storage: local filesystem

Use lightweight, mainstream libraries only when they clearly simplify the implementation.

## Search / retrieval
Retrieval uses a hybrid approach: SQLite FTS5 for keyword matching, combined with
optional embedding-based semantic search (vectors stored in SQLite as JSON).
The retrieval layer in `internal/service/retrieval.go` is isolated; additional strategies
(e.g., re-ranking, cross-encoder) can be plugged in without touching chat logic.
Embeddings are opt-in (disabled when `EMBEDDING_MODEL` is not configured).

## Documents
Support these document types:
- markdown
- text
- image

For images, store:
- file
- title
- note
- tags
- derived_text

Search should work against text content, note, tags, and derived_text.

## Memory
Persist only durable facts that are useful across chats.
Do not store transient chatter as memory.
Keep memory simple, but structure it so these categories can exist:
- semantic
- procedural
- episodic

## UI
Must include:
- project list
- project detail
- chat screen
- document / image add flow

The UI can be simple, but should be comfortable to use.
CLI-only UX is not acceptable.

## Shared design (snz-design)
The UI follows the shared design specification kept in the snz-design repository
(`serendipitynz/snz-design`). Check it out beside this repository as `../snz-design`.

- Before building or changing a screen, read the adoption guide (snz-design doc-16) and
  snz_studio's adoption record (snz-design doc-17), which lists what each screen already
  follows and its recorded exceptions.
- `frontend/src/styles/themes/snz-tokens.ts` is a generated copy. Do not edit it by hand;
  replace it with `node ../snz-design/tokens/vendor.mjs copy <version> snz-tokens.ts <path>`
  and check it with `vendor.mjs verify`.

## Implementation style
- Start from the smallest working version
- Prefer clear code over abstraction
- Avoid unnecessary layers
- Keep files reasonably small
- Use straightforward naming
- Leave concise notes for future extension points

## Delivery
Include:
- runnable app
- README
- sample env file
- sample seed data
- basic setup instructions

<!-- BACKLOG.MD GUIDELINES START -->
<!-- backlog.md-instructions-version: 1.50.1 -->
<CRITICAL_INSTRUCTION>

## Backlog.md Workflow

This project uses Backlog.md for task and project management.

Use `backlog <command> --help` before running unfamiliar commands. Help shows options, fields, and examples.

Do not edit Backlog task, draft, document, decision, or milestone markdown files directly. Use the `backlog` CLI so metadata, relationships, and history stay consistent.

</CRITICAL_INSTRUCTION>
<!-- BACKLOG.MD GUIDELINES END -->
