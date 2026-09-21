# SNZ Studio

> 日本語: [README.ja.md](README.ja.md)

A minimal local-LLM project workspace for personal use.
It offers an experience close to ChatGPT / Claude Projects, built out of nothing more than
**Wails v2 (Go core + OS-native WebView) + a React/Vite frontend + SQLite + the local
filesystem** — a standalone desktop app you install and launch.

## Features

- Create, list and view projects
- Per-project `documents`, `chats` and `memories`
- Register `markdown` / `text` / `image` documents
- Automatic document-category inference, with manual override
- SQLite FTS-based document / memory search
- Per-chat summary storage
- A minimal persistent-memory implementation
- Per-assistant-reply display of which `project` / `summary` / `document` / `memory` was referenced
- Multi-agent chats (chats where two or more participants speak in turn)
  - Per-participant display name, role prompt, endpoint and model (splitting endpoints lets you
    mix several LM Studio instances)
  - Turn rules: cycling the roster (`round_robin`) or nominating the next speaker (`manual`)
  - A scene prompt (topic, setting, world) shared by every participant
  - 7 bundled presets (debate, improv theatre and others) to start from, plus additional presets
    loaded from JSON files
  - A spectator view that advances one turn at a time or runs automatically, and lets you speak
    into the conversation as a human at any point
- Connects to OpenAI-compatible APIs
  - LM Studio
  - Ollama's OpenAI-compatible endpoint
  - Other compatible servers

## Structure

The Go backend is split into layers under `internal/`. The frontend stays plain React and talks
to the local Go `net/http` server on `127.0.0.1` (`/api`, `/files`) by absolute URL — a local
server rather than the Wails AssetServer bridge, so that SSE keeps working.

```text
main.go                Wails startup + embedded SPA
app.go                 App lifecycle / local API server startup / GetApiBase binding
internal/
  bootstrap/           data-path resolution, first-run migration, dev/prod switching
  config/              app-config.json + env defaults
  db/                  SQLite connection, schema, 10 migrations
  repository/          persistence for project / document / memory / chat / participant
  search/              Japanese tokenizer port + FTS query construction
  vector/              cosine similarity
  service/             retrieval / context / llm / embedding / summary / memory / review / turnengine
  preset/              bundled multi-agent presets (bundled/*.json via go:embed) + validation
  httpapi/             35 route handlers + SSE
presets/multi-agent/   additional, non-bundled presets (JSON; format described in that directory's README)
frontend/
  src/
    api/               HTTP client
    components/        shell + roster panel (ParticipantPanel)
    pages/             screens (ChatPage for single-assistant, MultiAgentChatPage for multi-agent)
    styles/            emotion styles
    wailsjs/           generated bindings (GetApiBase)
```

The layering is kept at this granularity:

- UI layer: `frontend/src/pages`
- API layer: `internal/httpapi`
- domain / service layer: `internal/service`
- persistence layer: `internal/repository`
- retrieval layer: `internal/service/retrieval.go`
- LLM integration layer: `internal/service/llmclient.go`
- multi-agent turn layer: `internal/service/turnengine.go` (independent of the single-assistant
  `chat.go`; they share the LLM client, the repositories and retrieval — a turn takes the project's
  documents and memories as background material through `turncontext.go`)

## Data model

SQLite holds the following minimum:

- `projects`
- `documents`
- `document_chunks`
- `document_chunks_fts`
- `chats`
- `messages`
- `chat_summaries`
- `memories`
- `memories_fts`
- `assistant_message_references`
- `participants` (multi-agent participants; removal is a `deleted_at` soft delete, so past
  messages keep their attribution)

`chats` carries the kind (`kind`), turn rule (`turn_rule`) and scene prompt (`scene_prompt`);
`messages` carries the speaker (`participant_id`). Existing chats stay `kind = 'assistant'`.

## Requirements

- Go 1.26.3 or newer (the `go` command fetches a missing version itself, so you do not need to
  install a specific one up front)
- Node 22 / pnpm (used for the frontend build; `wails` invokes it automatically)
- [Wails CLI v2](https://wails.io/) (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`)

> **About the Go version.** The x/tools used for binding generation is embedded in the Wails CLI
> binary and cannot be swapped out from go.mod, so building with a Go newer than the CLI can read
> makes binding generation fail with `internal error: package "math" without types was imported
> from ...`. For that reason, **launch `wails` through the pnpm scripts below rather than
> directly** — the script (`scripts/wails.mjs`) pins the exact version via `GOTOOLCHAIN`, so the
> result does not depend on which Go you happen to have installed.
>
> go.mod's `toolchain` directive is a **floor** (do not build below this), not a ceiling. A newer
> local Go is used as-is, so a bare `wails dev` is not pinned. The only way to enforce a ceiling is
> an exact `GOTOOLCHAIN`, which is what the pnpm script does.
>
> go.mod's `toolchain` is the single source of truth for the Go version; `scripts/wails.mjs` and
> CI both read it. Raise it together with a Wails CLI that can read it (go.mod's `toolchain` and
> `require`, plus the `go install` in `.github/workflows/build.yml`). Leaving the CLI behind
> reproduces the same failure, so do not ignore the `go.mod is using Wails 'x' but the CLI is 'y'`
> warning from `wails build`.

## Development (pnpm dev)

Run this at the repository root. The Go API (`127.0.0.1:8787`) and the frontend (Vite) start, and
the SPA is displayed in an OS-native WebView.

```bash
pnpm dev
```

- This is `scripts/wails.mjs dev` (it passes go.mod's `toolchain` as `GOTOOLCHAIN` and runs
  `wails dev`). A bare `wails dev` also starts, but then your local Go is used as-is and the
  caveat under "Requirements" applies.
- Development data is created under `./data` (relative to the cwd). `.env` (optional;
  `cp .env.example .env`) can override defaults such as `LLM_BASE_URL`, but normally you configure
  these from `Configuration` in the UI.
- Developing against the browser directly (`localhost:5173`) has been dropped, because non-GET
  requests do not reach the API. Use `pnpm dev`.

## Building (distributables)

There are two build stages, used for different purposes.

### 1. For verification (plain build)

```bash
pnpm build:app                                          # for the current OS
pnpm build:app -platform darwin/universal               # macOS universal (.app)
pnpm build:app -platform windows/amd64 -nsis -webview2 download
```

`build:app` is `scripts/wails.mjs build`; extra arguments are passed straight through to
`wails build`. The launcher is written in Node, so the same command works on Windows.

Artifacts land in `build/bin/` (`SNZ Studio.app` on macOS, `.exe` on Windows).

> **Note**: `wails build` alone does not bundle the built-in embedding stack (the llama.cpp sidecar
> and the model GGUF). Launching that `.app` reports `llama-server binary not found` for built-in
> embedding (it still runs with an external embedding endpoint, or with FTS only). A distributable
> that includes built-in embedding is produced by the signed build below.

### 2. For distribution (macOS: signing + notarization + bundled embedding)

The distributable macOS DMG is produced in one shot by a local script (the confirmed
signing-and-notarization recipe).

```bash
scripts/build-mac-signed.sh
```

It runs `wails build` (default `darwin/universal`) → staging and signing the llama.cpp sidecar
(`llama-server` + dylibs) → staging the model GGUF → signing the `.app` / `.dmg` with a hardened
runtime and a secure timestamp → `notarytool submit --wait` → `stapler staple`, producing
`build/bin/SNZ-Studio.dmg`.

You need:

- a "Developer ID Application" certificate (installed in the login keychain)
- a saved notarytool profile (default name `snzstudio`; created once with
  `xcrun notarytool store-credentials`)
- `data/models/ruri-v3-30m-q8_0.gguf` (regenerated by the script in the next section)

Main env overrides: `DEVELOPER_ID` / `NOTARY_PROFILE` / `PLATFORM` / `LLAMA_RELEASE` /
`SIDECAR_ARCH` / `MODEL_SRC`.

> Windows signing is not supported yet (unsigned distribution for now). CI
> (`.github/workflows/build.yml`) is a scaffold: `workflow_dispatch` only, with signing deferred
> behind secrets gates.

### Regenerating the built-in embedding model (GGUF)

Built-in embedding uses `cl-nagoya/ruri-v3-30m` (ModernBERT-Ja, 256 dimensions, Apache-2.0)
converted to GGUF with llama.cpp and quantized to q8_0; the file is bundled into the `.app` and
unpacked into `models/` under the user's data directory on first launch. That GGUF
(`data/models/ruri-v3-30m-q8_0.gguf`, about 42MB) is outside git because of its size, so the
following script regenerates it reproducibly.

```bash
scripts/build-ruri-gguf.sh
```

Using llama.cpp `b9437`'s source (the converter) and release (`llama-quantize`), a pinned HF
revision, and pinned Python dependencies (torch / transformers / sentencepiece / gguf), it runs
HF download → converter patch (SentencePiece) → f16 → q8_0 → sha256 verification → installation
into `data/models/`, idempotently (each stage is skipped when its output already exists). It aborts
when the sha256 does not match the one pinned in `internal/embed/modelspec.go`.

## Data location and migration

- Distributed (prod) builds store data under the OS user-configuration directory.
  - macOS: `~/Library/Application Support/snz-studio`
  - Windows: `%AppData%\snz-studio`
  - containing `app.sqlite` / `uploads/` / `app-config.json`.
- To carry over `data/` from the old Node version, point `SNZ_MIGRATE_FROM` at the source on first
  launch (it runs only on a first launch into an empty destination, and does not modify the source).

```bash
SNZ_MIGRATE_FROM="/path/to/old/data" "build/bin/SNZ Studio.app/Contents/MacOS/SNZ Studio"
```

## LLM connection

An OpenAI-compatible API is assumed. The main `.env` settings are:

- `LLM_BASE_URL`
- `LLM_MODEL`
- `REVIEW_BASE_URL`
- `REVIEW_MODEL`
- `LLM_API_KEY`
- `LLM_TIMEOUT_MS`
- `EMBEDDING_BASE_URL`
- `EMBEDDING_MODEL`
- `EMBEDDING_API_KEY`
- `EMBEDDING_TIMEOUT_MS`
- `DEBUG_CHAT_FLOW`
- `DEBUG_RETRIEVAL`

For example:

- LM Studio: `LLM_BASE_URL=http://127.0.0.1:1234/v1`
- Ollama's OpenAI-compatible endpoint: substitute that URL

Set `EMBEDDING_MODEL` to use embeddings. Left unset, retrieval runs on FTS alone; set, document and
memory retrieval becomes a hybrid of `FTS + embedding rerank`.

`DEBUG_CHAT_FLOW` / `DEBUG_RETRIEVAL` are accepted for compatibility, but the Go version keeps
logging minimal and emits no verbose traces.

Endpoints, models, `LLM Response Format` and the review endpoint / model can be updated from
`Configuration` on the Dashboard. Values saved from the UI are stored in `app-config.json` under the
app's data directory, take precedence over `.env`, and apply immediately. For thinking-style models
such as `llm-jp-4-8b-thinking`, choosing `LLM-jp Thinking` strips the internal reasoning / tagged
response and shows only the final answer.

The app itself works even when no local LLM is running. Chat replies then fall back to a canned
message, which is still useful for checking which references were selected.

## Implementation approach

- No vector database
- Retrieval is SQLite FTS by default, with optional hybrid rerank via OpenAI-compatible embeddings
- Documents are classified as `world / character / rule / plot / timeline / index / story / misc`,
  used to tune retrieval priority
- Vectors are stored in SQLite as JSON; no external vector database
- Past chats are not passed in full every time — `chat_summaries` plus recent messages carry them
- Memories are meant to hold durable facts only
- Memory extraction is a lightweight rule-based heuristic in this initial implementation
- A multi-agent chat is one request = one turn (no resident driver job; automatic advancing is a
  loop in the frontend). A turn in flight runs to completion and is saved even if the client
  disconnects, so turn boundaries are the only granularity at which it can be stopped

## API highlights

- `GET /api/projects`
- `POST /api/projects`
- `GET /api/projects/:projectId`
- `POST /api/projects/:projectId/chats`
- `POST /api/projects/:projectId/memories`
- `POST /api/projects/:projectId/documents`
- `GET /api/chats/:chatId`
- `POST /api/chats/:chatId/messages`
- `GET /api/multi-agent-presets`
- `GET / POST /api/chats/:chatId/participants`
- `PATCH / DELETE /api/participants/:participantId`
- `POST /api/chats/:chatId/turns/stream` (runs one turn over SSE; a duplicate call while one is
  running returns 409)
- `GET /api/messages/:messageId/memory-draft` / `POST /api/messages/:messageId/memory` (save one
  multi-agent utterance as a project memory; a multi-agent chat never extracts memories on its own)

## Future extension points

- Tuning hybrid-retrieval weights and improving the semantic-only fallback
- Moving chat-summary updates onto the LLM
- Moving memory extraction onto the LLM
- Document edit / delete UI
- A rerank layer
- Better manual-annotation UX for image documents
- TRPG support for multi-agent chats (per-chat state, dice, structured-output checks), speaker
  nomination by a moderator model, and generation cancellation
  ([design doc](docs/multi-agent-chat-design.md) §7, in Japanese)

## License

The app itself is under the MIT License ([LICENSE](LICENSE)).

Copyright notices and full license texts for the third-party works bundled or ported here — the
TinySegmenter-derived code in `internal/search`, and the llama.cpp and ruri-v3-30m bundled into the
distributables — are collected in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Caveats

- No authentication; a single user is assumed
- No PDF / OCR / vector search / Electron
- A minimal build that prioritizes development speed and readability

## Documentation

Most design and specification documents under `docs/` are written in Japanese; those with an
English counterpart use the `*.md` (English) / `*.ja.md` (Japanese) pair convention.
