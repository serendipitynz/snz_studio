#!/usr/bin/env bash
# Regenerate data/models/ruri-v3-30m-q8_0.gguf from scratch, reproducibly.
#
# The q8_0 GGUF is gitignored (data/ is ignored, ~42MB binary), so this script is the
# canonical way to recreate it on a fresh machine before `scripts/build-mac-signed.sh`
# bundles it into the .app. The full pipeline is:
#
#   1. fetch llama.cpp SOURCE   @ $LLAMA_RELEASE  (for convert_hf_to_gguf.py)
#   2. fetch llama.cpp RELEASE  @ $LLAMA_RELEASE  (for the llama-quantize binary)
#   3. create a venv + pip install the converter's PINNED requirements
#   4. patch conversion/bert.py: ModernBertModel.set_vocab uses SentencePiece, not GPT2
#   5. download cl-nagoya/ruri-v3-30m @ $HF_REVISION  (SentencePiece/ModernBERT-Ja)
#   6. convert  -> ruri-v3-30m-f16.gguf   (--outtype f16)
#   7. quantize -> ruri-v3-30m-q8_0.gguf  (Q8_0)
#   8. verify size + sha256 against the values pinned in internal/embed/modelspec.go
#   9. copy the verified q8_0 into data/models/
#
# Every stage is idempotent: it skips when its output is already present (e.g. if an
# f16 already exists, only quantize runs). Delete $WORKDIR to force a clean rebuild.
#
# Reproducibility caveat: byte-for-byte sha256 reproduction depends on the pinned
# llama.cpp release AND the pinned Python deps. If upstream tooling drifts, the final
# sha256 may differ; the script then FAILS with the new values so you can decide to
# either pin a matching toolchain or update modelspec.go's SHA256/SizeBytes.
#
# Usage:
#   scripts/build-ruri-gguf.sh
# Env overrides:
#   LLAMA_RELEASE   llama.cpp tag for source + quantize binary. Default: b9437.
#   HF_REPO         source model repo.   Default: cl-nagoya/ruri-v3-30m.
#   HF_REVISION     pinned commit/branch. Default: 24899e5de370b56d179604a007c0d727bf144504.
#   SIDECAR_ARCH    release binary arch (arm64|x64). Default: host arch.
#   WORKDIR         scratch dir.          Default: build/model-build.
#   OUTPUT          final path.           Default: data/models/ruri-v3-30m-q8_0.gguf.
#   PYTHON          python interpreter.   Default: python3.
set -euo pipefail

cd "$(dirname "$0")/.."

LLAMA_RELEASE="${LLAMA_RELEASE:-b9437}"
HF_REPO="${HF_REPO:-cl-nagoya/ruri-v3-30m}"
HF_REVISION="${HF_REVISION:-24899e5de370b56d179604a007c0d727bf144504}"
WORKDIR="${WORKDIR:-build/model-build}"
OUTPUT="${OUTPUT:-data/models/ruri-v3-30m-q8_0.gguf}"
PYTHON="${PYTHON:-python3}"

# Absolutize WORKDIR/OUTPUT (cwd is the repo root here) so every derived path stays
# valid even when a stage runs a tool from another directory.
abspath() { case "$1" in /*) printf '%s\n' "$1" ;; *) printf '%s/%s\n' "$(pwd)" "$1" ;; esac; }
WORKDIR="$(abspath "$WORKDIR")"
OUTPUT="$(abspath "$OUTPUT")"

# Expected artifact identity — keep in sync with internal/embed/modelspec.go
# (RuriV3_30m.SHA256 / SizeBytes). The script verifies the rebuilt q8_0 against these.
EXPECT_SHA256="2a6cb2d9889140cd214bc4eaee14114f276a52afcf0a2fe65fae3d467f7480fe"
EXPECT_SIZE="41569120"

case "$(uname -m)" in
  arm64|aarch64) ARCH_DEFAULT=arm64 ;;
  x86_64)        ARCH_DEFAULT=x64 ;;
  *)             ARCH_DEFAULT=arm64 ;;
esac
SIDECAR_ARCH="${SIDECAR_ARCH:-$ARCH_DEFAULT}"

SRC_DIR="$WORKDIR/llama.cpp-$LLAMA_RELEASE"
TOOLS_DIR="$WORKDIR/tools-$LLAMA_RELEASE-$SIDECAR_ARCH"
VENV_DIR="$WORKDIR/venv"
HF_DIR="$WORKDIR/hf/$(basename "$HF_REPO")"
F16="$WORKDIR/ruri-v3-30m-f16.gguf"
Q8="$WORKDIR/ruri-v3-30m-q8_0.gguf"

sha256_of() { shasum -a 256 "$1" | awk '{print $1}'; }
size_of()   { stat -f%z "$1"; }
# A q8_0 already matching the pinned identity is "verified": size first (cheap), then sha.
verified()  { [[ -f "$1" && "$(size_of "$1")" == "$EXPECT_SIZE" && "$(sha256_of "$1")" == "$EXPECT_SHA256" ]]; }

echo "==> Target: $OUTPUT (llama.cpp $LLAMA_RELEASE, $HF_REPO@${HF_REVISION:0:12}, arch $SIDECAR_ARCH)"
if verified "$OUTPUT"; then
  echo "    already present and verified — nothing to do."
  exit 0
fi
mkdir -p "$WORKDIR"

# ---------------------------------------------------------------------------- #
echo "==> [1/9] llama.cpp source ($LLAMA_RELEASE) for the converter"
if [[ -f "$SRC_DIR/convert_hf_to_gguf.py" ]]; then
  echo "    cached at $SRC_DIR"
else
  TMP="$(mktemp -d)"
  curl -fsSL "https://github.com/ggml-org/llama.cpp/archive/refs/tags/${LLAMA_RELEASE}.tar.gz" -o "$TMP/src.tar.gz"
  tar xzf "$TMP/src.tar.gz" -C "$TMP"
  EXTRACTED="$(find "$TMP" -maxdepth 1 -type d -name 'llama.cpp-*' | head -1)"
  [[ -n "$EXTRACTED" ]] || { echo "ERROR: could not find extracted source dir" >&2; exit 1; }
  rm -rf "$SRC_DIR"
  mv "$EXTRACTED" "$SRC_DIR"
  rm -rf "$TMP"
  echo "    extracted to $SRC_DIR"
fi

# ---------------------------------------------------------------------------- #
echo "==> [2/9] llama.cpp release ($LLAMA_RELEASE) for llama-quantize"
QUANTIZE="$(find "$TOOLS_DIR" -name 'llama-quantize' -type f 2>/dev/null | head -1 || true)"
if [[ -n "$QUANTIZE" ]]; then
  echo "    cached at $QUANTIZE"
else
  TMP="$(mktemp -d)"
  TARBALL="llama-${LLAMA_RELEASE}-bin-macos-${SIDECAR_ARCH}.tar.gz"
  curl -fsSL "https://github.com/ggml-org/llama.cpp/releases/download/${LLAMA_RELEASE}/${TARBALL}" -o "$TMP/rel.tar.gz"
  tar xzf "$TMP/rel.tar.gz" -C "$TMP"
  SRCQ="$(find "$TMP" -name 'llama-quantize' -type f | head -1)"
  [[ -n "$SRCQ" ]] || { echo "ERROR: llama-quantize not found in $TARBALL" >&2; exit 1; }
  rm -rf "$TOOLS_DIR"
  mkdir -p "$TOOLS_DIR"
  # Copy the whole bin dir so llama-quantize finds its sibling dylibs via DYLD_LIBRARY_PATH.
  cp -R "$(dirname "$SRCQ")"/. "$TOOLS_DIR"/
  rm -rf "$TMP"
  QUANTIZE="$TOOLS_DIR/llama-quantize"
  echo "    staged llama-quantize into $TOOLS_DIR"
fi

# ---------------------------------------------------------------------------- #
echo "==> [3/9] Python venv + pinned converter requirements"
if [[ ! -x "$VENV_DIR/bin/python" ]]; then
  "$PYTHON" -m venv "$VENV_DIR"
fi
VPY="$VENV_DIR/bin/python"
if [[ ! -f "$VENV_DIR/.requirements-installed" ]]; then
  "$VPY" -m pip install --upgrade pip >/dev/null
  "$VPY" -m pip install -r "$SRC_DIR/requirements/requirements-convert_hf_to_gguf.txt"
  "$VPY" -m pip install huggingface_hub
  touch "$VENV_DIR/.requirements-installed"
  echo "    installed (torch/transformers/sentencepiece/gguf/protobuf + huggingface_hub)"
else
  echo "    already installed (delete $VENV_DIR to reinstall)"
fi

# ---------------------------------------------------------------------------- #
echo "==> [4/9] Patch ModernBertModel.set_vocab (SentencePiece, not GPT2)"
if [[ -f "$SRC_DIR/.ruri-patched" ]]; then
  echo "    already patched"
else
  # ruri is a SentencePiece(Unigram) ModernBERT-Ja model. Upstream's
  # ModernBertModel.set_vocab assumes an English BPE vocab and calls _set_vocab_gpt2(),
  # which raises 'BPE pre-tokenizer was not recognized'. Swap ONLY that call (scoped to
  # the ModernBertModel class — the file has other _set_vocab_gpt2() callers) for
  # _set_vocab_sentencepiece(); the add_bos/eos/sep flags already in the method stay.
  "$VPY" - "$SRC_DIR/conversion/bert.py" <<'PY'
import re, sys
path = sys.argv[1]
src = open(path, encoding="utf-8").read()
marker = "class ModernBertModel(BertModel):"
i = src.find(marker)
if i < 0:
    sys.exit("PATCH FAILED: 'class ModernBertModel(BertModel):' not found — converter changed; review the patch.")
head, tail = src[:i], src[i:]
new_tail, n = re.subn(r"self\._set_vocab_gpt2\(\)", "self._set_vocab_sentencepiece()", tail, count=1)
if n != 1:
    sys.exit("PATCH FAILED: expected exactly one self._set_vocab_gpt2() inside ModernBertModel; found %d." % n)
open(path, "w", encoding="utf-8").write(head + new_tail)
print("    patched conversion/bert.py")
PY
  touch "$SRC_DIR/.ruri-patched"
fi

# ---------------------------------------------------------------------------- #
echo "==> [5/9] Download $HF_REPO @ $HF_REVISION"
if [[ -f "$HF_DIR/config.json" ]]; then
  echo "    cached at $HF_DIR"
else
  # Use the huggingface_hub Python API (snapshot_download) rather than the CLI: the
  # `huggingface-cli` shim is deprecated/removed in recent versions (renamed to `hf`),
  # and the library API is stable across those renames.
  "$VPY" - "$HF_REPO" "$HF_REVISION" "$HF_DIR" <<'PY'
import sys
from huggingface_hub import snapshot_download
repo, rev, dest = sys.argv[1:4]
snapshot_download(repo_id=repo, revision=rev, local_dir=dest)
PY
  echo "    downloaded to $HF_DIR"
fi

# ---------------------------------------------------------------------------- #
echo "==> [6/9] Convert -> f16 GGUF"
if [[ -f "$F16" ]]; then
  echo "    f16 already present at $F16"
else
  # Run by absolute path (no cd): Python puts the script's dir on sys.path[0], so the
  # in-tree `conversion` package and gguf-py (inserted via __file__) import correctly.
  "$VPY" "$SRC_DIR/convert_hf_to_gguf.py" "$HF_DIR" --outtype f16 --outfile "$F16"
  echo "    wrote $F16 ($(size_of "$F16") bytes)"
fi

# ---------------------------------------------------------------------------- #
echo "==> [7/9] Quantize -> q8_0 GGUF"
if [[ -f "$Q8" ]]; then
  echo "    q8_0 already present at $Q8"
else
  DYLD_LIBRARY_PATH="$(dirname "$QUANTIZE")" "$QUANTIZE" "$F16" "$Q8" Q8_0
  echo "    wrote $Q8 ($(size_of "$Q8") bytes)"
fi

# ---------------------------------------------------------------------------- #
echo "==> [8/9] Verify size + sha256 against modelspec.go"
GOT_SIZE="$(size_of "$Q8")"
GOT_SHA="$(sha256_of "$Q8")"
echo "    size   got=$GOT_SIZE   expect=$EXPECT_SIZE"
echo "    sha256 got=$GOT_SHA"
echo "           expect=$EXPECT_SHA256"
if [[ "$GOT_SIZE" != "$EXPECT_SIZE" || "$GOT_SHA" != "$EXPECT_SHA256" ]]; then
  cat >&2 <<EOF

ERROR: rebuilt q8_0 does NOT match the pinned identity in internal/embed/modelspec.go.
       This usually means the llama.cpp release or the Python converter deps drifted.
       Options:
         - pin a toolchain that reproduces the committed sha256, OR
         - if this artifact is acceptable, update internal/embed/modelspec.go:
             SHA256:    "$GOT_SHA"
             SizeBytes: $GOT_SIZE
           (ModelID stays "ruri-v3-30m" — sha256 only gates download/seed integrity.)
       The unverified q8_0 was left at: $Q8 (not copied to $OUTPUT).
EOF
  exit 1
fi

# ---------------------------------------------------------------------------- #
echo "==> [9/9] Install -> $OUTPUT"
mkdir -p "$(dirname "$OUTPUT")"
cp "$Q8" "$OUTPUT"
echo "    done: $OUTPUT (verified)"
