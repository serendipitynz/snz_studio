# Project Specification

## Purpose

A project is the top-level workspace unit in SNZ Studio.
It groups shared context for a single creative or research activity, such as a novel, translation task, or technical project.

Each project provides:

- shared documents
- shared memories
- multiple chats
- a project-level system prompt

The intended UX is similar to ChatGPT / Claude Projects, but limited to a single user on a local machine.

## Data Model

A project stores:

- `id`
- `title`
- `description`
- `system_prompt`
- `created_at`
- `updated_at`

Related records:

- `documents`
- `chats`
- `memories`

Deleting a project removes its related chats, documents, summaries, messages, memories, and uploaded files.

## Project-Level Context

Project-level context is assembled from several sources:

1. project title
2. project description
3. project system prompt
4. project memories
5. project documents
6. chat summary
7. recent chat messages

The project itself is always represented as one reference source in assistant answers.

## Documents

Projects can contain these document types:

- `markdown`
- `text`
- `image`

Each document also has a category:

- `world`
- `character`
- `rule`
- `plot`
- `timeline`
- `index`
- `story`
- `misc`

Categories are inferred automatically on creation and can be changed manually later.

Document retrieval uses:

- SQLite FTS5 keyword search
- optional embedding-based reranking and fallback

Large writing projects are expected to keep source material in project documents rather than in memories.

## Chats

Each chat belongs to exactly one project.

A project can have multiple chats.
Chats share the project's documents and memories, but keep their own:

- messages
- summary
- reference history

Chat titles may be empty at creation time.
If a title is empty, the system generates one automatically after the first assistant turn.

## Memories

Memories belong to a project and are shared across its chats.

They are intended for durable, reusable context rather than raw source material.

Kinds:

- `procedural`
- `semantic`
- `episodic`

Metadata:

- `source`: `manual`, `chat`, or `organized`
- `locked`: protects a memory from organizer rewrite/remove, but not from manual edit/delete

## Retrieval Behavior

At answer time, the backend assembles context in this order:

1. project settings
2. procedural memories
3. chat summary
4. relevant documents
5. relevant memories
6. recent messages

Document retrieval is category-aware.
For example:

- setting checks prefer `world`, `index`, `timeline`
- writing assistance prefers `rule`, `story`, `character`
- plot questions prefer `plot`, `timeline`

## UI Behavior

The project detail screen includes:

- project title editing
- new chat creation
- document upload and browsing
- memory management
- system prompt editing
- chat list

The right pane acts as a project asset inspector.

## Configuration Relationship

Projects do not store model endpoint settings.
LLM and embedding configuration are workspace-level settings stored in `data/app-config.json`.

Projects only store content and instructions specific to the project itself.
