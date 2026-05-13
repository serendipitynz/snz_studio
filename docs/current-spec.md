# SNZ Studio Current Specification

Last updated: 2026-05-09

This document summarizes the current behavior of SNZ Studio for future reimplementation in Go or conversion into a desktop application.

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

- Frontend: React + Vite + TypeScript + Emotion
- Backend: Express + TypeScript
- Database: SQLite via `better-sqlite3`
- Storage: local filesystem
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

Empty titles are allowed and are auto-generated after the first assistant response.

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

- new chat
- documents card

Right pane:

- system prompt
- memories
- chats

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
- `schema_migrations`

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
