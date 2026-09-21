# SNZ Studio Current Specification

Last updated: 2026-09-19

This document summarizes the current behavior of SNZ Studio, now implemented as a Wails v2 desktop app (Go backend + React frontend).

## Goal

SNZ Studio is a single-user local LLM workspace for:

- fiction writing assistance
- translation assistance
- document-backed chat
- project-scoped memory and summaries

The UX target is a lightweight local analogue of ChatGPT / Claude Projects.

## Non-goals

- multi-user
- auth
- external infrastructure
- vector database
- PDF / OCR
- Electron-specific architecture

## Current stack

- Desktop shell: Wails v2 (Go core + OS-native WebView)
- Frontend: React + Vite + TypeScript + Emotion
- Backend: Go + standard `net/http` (loopback `127.0.0.1`, serving `/api` and `/files`)
- Database: SQLite via `modernc.org/sqlite` (pure Go, FTS5 / bm25)
- Storage: local filesystem under the OS user config dir (`./data` in dev)
- LLM API: OpenAI-compatible endpoint
- Embeddings: optional OpenAI-compatible endpoint

## Top-level concepts

### Projects

A project contains:

- title
- description
- system prompt
- documents
- memories
- chats

Projects also have persistent ordering (`sort_order`) and chat counts.

### Documents

Supported document types:

- markdown
- text
- image

Current document categories:

- `world`
- `character`
- `rule`
- `plot`
- `timeline`
- `index`
- `story`
- `misc`

Categories are inferred on create and can be edited manually.

### Chats

A chat belongs to exactly one project and contains:

- messages
- summary
- assistant references
- title
- temporary flag
- kind: `assistant` (single assistant) or `multi_agent` (multi-agent conversation)

Empty titles are allowed and are auto-generated after the first assistant response.

### Multi-agent conversations

A multi-agent conversation is a chat of kind `multi_agent`: two or more participants speak in turn,
each through its own endpoint and model. It is a kind of chat, not a separate top-level concept — it
lives in the same `chats` table and under the same project.

A multi-agent conversation has:

- participants: display name, role prompt, endpoint, model, roster order, and whether the
  participant is given the project material (on by default)
- turn rule: `round_robin` (cycle the roster) or `manual` (nominate each speaker)
- scene: text prefixed to every participant's system prompt (topic, setting, world)

The roster is the set of participants still on the conversation. Removing a participant is a soft
delete: the row stays, so past messages keep their speaker name, and the round-robin cycle keeps a
starting point.

Presets fill participants, turn rule and scene in one step. Seven presets ship with the app; further
presets are applied by loading a JSON file of the same shape. A preset can be applied when the
conversation is created, and afterwards from the organisation panel for as long as the conversation
has no messages — which replaces the roster, the turn rule and the scene. Once something has been
said the preset is refused, since the transcript would be left naming speakers the conversation no
longer has. An applied preset leaves no link behind — everything is edited from the organisation
panel afterwards.

The user is not a participant. Human messages are stored as `user` messages with no participant, so
the user can speak into the conversation at any point (adding a topic, heckling) without taking a turn.

### Temporary chats

Temporary chats are intended for drafts and disposable exploration.

They:

- do not create automatic memories
- ignore explicit “remember this” triggers
- are excluded from memory organizer analysis

They still:

- store messages
- store summaries
- can be referenced explicitly from other chats

### Memories

Memories store durable project-level facts, not raw conversation logs.

Kinds:

- `procedural`
- `semantic`
- `episodic`

Metadata:

- `source`: `manual` / `chat` / `organized`
- `locked`: protected from organizer rewrite/remove

## Chat generation flow

Current assistant turn flow:

1. load chat and project
2. assemble context
3. save user message
4. optionally extract memories
5. generate assistant response
6. save references
7. update summary
8. optionally auto-title the chat

## Multi-agent turn flow

One API request runs exactly one turn. There is no long-running conversation job on the server;
auto-advance is the frontend calling the next turn repeatedly.

A turn:

1. takes the conversation's turn lock (a second concurrent turn gets 409)
2. picks the speaker (round-robin: the next roster entry after the last participant message; manual:
   the nominated participant)
3. checks the participant's endpoint and loads the model
4. builds the prompt from the participant's point of view: system = project material (project
   description, matched document passages, matched memories; see below) + scene + role prompt + role
   reminder; history mapped to `assistant` for the participant's own past messages and `user` for
   everyone else's, prefixed with the speaker's display name; the prompt ends on the message this
   participant has to answer
5. streams the utterance and stores it as an `assistant` message carrying `participant_id`, together
   with the references the project material was built from (one transaction)

Current limits, accepted as behavior:

- history is truncated to the most recent 30 messages; there is no summary-based compression
- a turn in progress is never interrupted — it finishes generating and is stored even if the client
  disconnects, so stopping auto-advance only takes effect at the turn boundary
- recovery after a disconnect is re-reading the stored messages; missed deltas are not replayed

The multi-agent flow is separate from the single-assistant flow: it shares the LLM client, the
persistence layer and the retrieval service, and does not use the chat summary or the rest of the
single-assistant prompt context (project system prompt, unconditional procedural memories, quote mode,
referenced chats). Project material is retrieved once per turn with a query made of the latest
utterance, the two before it and the head of the scene (the scene alone on the opening turn), capped
at 2 documents × 2 passages, 3 memories and 2,000 characters in total. The references used are stored
with the participant's message and shown under it as on the single-assistant chat screen. A failed
search leaves the turn without material rather than failing it. A participant that is not given the
project material gets only the common project material on its turns — the documents and memories
marked "share with every participant" — and never the project description; the narrowing happens in
SQL, before ranking, so the turn's small budget is spent on rows it may have. That is how the players
read the rules and the world while one speaker holds the scenario. Being given the material is on by
default, and nothing is shared with everyone by default, so both sides of who reads what take an
explicit decision. On a `multi_agent` chat the existing
message routes store the user's message without generating a reply.

## Context assembly

Current context sources:

- project title / description / system prompt
- current chat summary
- selected procedural memories
- relevant documents
- relevant memories
- recent messages
- optional referenced chat summary / recent messages

## Retrieval

Current retrieval is hybrid:

- SQLite FTS5 keyword search
- optional embedding rerank
- optional semantic-only fallback

Document retrieval is category-aware and intent-aware.

Examples:

- reference queries prioritize `world`, `index`, `timeline`
- writing queries prioritize `rule`, `story`, `character`
- plot queries prioritize `plot`, `timeline`
- translation queries lean toward `rule`, `story`, `index`

## Explicit references

### Explicit document reference

If a document title is explicitly mentioned, retrieval can prioritize it.

Current behaviors include:

- quote mode
- multiple matched chunks
- conditional full-document inclusion

### Explicit chat reference

If a user explicitly mentions another chat title, that chat can be included in context.

Current inclusion:

- referenced chat summary
- referenced chat recent messages

## Memory behavior

### Manual memory

Created from the project detail memory modal.

- user selects kind
- user enters content
- title is auto-generated
- manual memories default to `locked = true`

### Automatic memory extraction

Automatic extraction only considers user messages.

Current rules:

- questions are ignored
- long text is ignored
- transient requests are ignored
- plain `...してください` is no longer treated as durable procedural memory

### Explicit remember trigger

Phrases like:

- “覚えて”
- “メモリに保存して”
- “今後の前提にして”

trigger a dedicated memory extraction path.

## Streaming

Streaming is currently supported.

Current persistence model:

- an empty assistant message row is created before generation starts
- each delta updates `messages.content`
- finalization stores:
  - final content
  - response time
  - output tokens
  - tokens per second
  - model name
  - references
  - updated summary

This allows the database to reflect in-progress assistant output even if the chat screen is left during streaming.

## Review

Each assistant message can be reviewed separately.

Conceptually:

- generation model = author
- review model = editor

Review uses:

- the target assistant message
- assembled project context
- retrieved references

Review is streamed to the UI and rendered as markdown.

## Configuration

Workspace-level configuration currently includes:

- LLM endpoint
- LLM model
- LLM response format
- review endpoint
- review model
- embedding endpoint
- embedding model

Persisted in:

- `data/app-config.json`

`.env` acts as the initial/default source, but saved app config overrides it.

## Supported response formats

Current values:

- `standard`
- `llm_jp_thinking`

`llm_jp_thinking` strips tagged reasoning output and stores only the final user-facing answer.

## UI structure

### Dashboard

- project list
- drag-and-drop reorder
- project creation
- configuration display and editing

### Sidebar

- home
- project list
- chat list
- quick chat creation

Temporary chats use a `⏱️` prefix.

### Project detail

Center:

- new chat, with the chat kind and (for a multi-agent conversation) a preset selector grouped by
  situation, plus loading a preset JSON file
- documents card

Right pane:

- system prompt
- memories
- chats

### Multi-agent chat screen

Center:

- transcript, each utterance labelled with the speaker's display name and model
- streaming output for the turn in progress
- advance one turn / start and stop auto-advance / (manual rule) nominate the next speaker
- a composer for speaking into the conversation as the user

Right pane:

- organisation panel: participant CRUD with endpoint + model selection and a connection check,
  the per-participant project-material switch, roster order, turn rule, scene, and — while the
  conversation has no messages — applying a preset (collapsed by default)

Removed participants are listed separately from the roster, since their past utterances remain.

### Chat screen

Center:

- message list
- streaming output
- auto-growing composer
- document insert selector
- document add modal

Right pane:

- collapsible context inspector

Assistant footer shows:

- review button
- copy button
- timestamp
- response metrics
- model name

### Memory modal

- add memory
- lock / unlock
- delete
- organize

## Database

Main tables:

- `projects`
- `documents`
- `document_chunks`
- `document_chunks_fts`
- `document_chunk_embeddings`
- `chats`
- `messages`
- `chat_summaries`
- `memories`
- `memories_fts`
- `memory_embeddings`
- `assistant_message_references`
- `participants`
- `schema_migrations`

`chats` carries `kind` / `turn_rule` / `scene_prompt`, and `messages` carries `participant_id`
(null for user and single-assistant messages).

## API responsibilities

Current API groups:

- project CRUD
- document CRUD
- chat CRUD
- message send / stream
- memory CRUD
- memory organization
- review
- workspace configuration
- participant CRUD (soft delete)
- multi-agent turn streaming
- bundled preset listing
- applying a preset to a multi-agent conversation that has no messages

## Debugging

Supported debug flags:

- `DEBUG_CHAT_FLOW=1`
- `DEBUG_RETRIEVAL=1`

These enable staged logs for chat flow, context assembly, retrieval, and embeddings.

## Reimplementation boundaries to preserve

The most important boundaries to keep in a Go rewrite are:

- UI layer
- API / application orchestration layer
- persistence layer
- retrieval layer
- LLM integration layer

Most importantly, preserve the current information split:

- documents = source material
- memories = durable distilled facts/rules
- summaries = compressed per-chat conversation state
