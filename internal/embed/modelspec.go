// Package embed manages the bundled embedding model. On first launch it downloads
// the ruri-v3-30m GGUF and runs a local llama.cpp llama-server sidecar that serves
// an OpenAI-compatible /v1/embeddings endpoint, so the Go binary stays CGO-free.
//
// The package depends only on the standard library and is decoupled from
// internal/config and the service layer: the Manager reports readiness/loss through
// callbacks, which the HTTP layer wires to config's internal-embedding overlay.
package embed

// ModelSpec describes a downloadable embedding GGUF and how to verify and serve it.
type ModelSpec struct {
	// ModelID is the logical id stored in the embeddings `model` column and sent to
	// the sidecar. Changing it (or the prefixes/quantization) invalidates stored
	// vectors and requires a full RebuildAll.
	ModelID   string
	FileName  string // on-disk name under <dataDir>/models
	URL       string // direct download URL; pin to an immutable revision
	SHA256    string // lowercase hex sha256 of the file (verified after download)
	SizeBytes int64  // exact size; a cheap pre-check before hashing
	Dim       int    // embedding dimension; the sidecar probe asserts this

	// ContextLength is the model's trained maximum input length in tokens. The
	// sidecar sizes its context and batches to it: llama-server embeds a whole input
	// in one physical batch and rejects anything longer than -ub.
	ContextLength int

	// QueryPrefix/DocumentPrefix implement ruri's asymmetric (1+3) retrieval scheme.
	// They are baked into the stored vectors, so changing them requires RebuildAll.
	QueryPrefix    string
	DocumentPrefix string
}

// RuriV3_30m is the default bundled model: cl-nagoya/ruri-v3-30m converted to GGUF
// (the converter needs a one-line SentencePiece-vocab patch, applied host-side at
// build time) and quantized to q8_0. ModernBERT-Ja, 256-dim, mean pooling.
//
// Packaged builds bundle this GGUF inside the app (scripts/build-mac-signed.sh stages
// it into Contents/Resources; seedBundledModel copies it into the models dir on first
// launch), so the URL below is only a fallback for unbundled/dev builds.
//
// SHA256/SizeBytes identify the reproducible q8_0 artifact built by
// scripts/build-ruri-gguf.sh (llama.cpp b9437 + the pinned converter deps; the patched
// set_vocab writes add_bos/eos/sep=True per ruri's tokenizer config). Re-run that
// script to regenerate it byte-for-byte; update these two fields if the toolchain drifts.
//
// TODO(track-b): URL is a placeholder — fill it in only if/when the q8_0 GGUF is also
// hosted at an immutable location (e.g. a pinned Hugging Face revision) as a download
// fallback; bundling (above) is the primary distribution path and needs no URL.
var RuriV3_30m = ModelSpec{
	ModelID:        "ruri-v3-30m",
	FileName:       "ruri-v3-30m-q8_0.gguf",
	URL:            "https://huggingface.co/REPLACE_OWNER/ruri-v3-30m-GGUF/resolve/REPLACE_REVISION/ruri-v3-30m-q8_0.gguf",
	SHA256:         "2a6cb2d9889140cd214bc4eaee14114f276a52afcf0a2fe65fae3d467f7480fe",
	SizeBytes:      41569120,
	Dim:            256,
	ContextLength:  8192,
	QueryPrefix:    "検索クエリ: ",
	DocumentPrefix: "検索文書: ",
}
