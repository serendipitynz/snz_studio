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

	// QueryPrefix/DocumentPrefix implement ruri's asymmetric (1+3) retrieval scheme.
	// They are baked into the stored vectors, so changing them requires RebuildAll.
	QueryPrefix    string
	DocumentPrefix string
}

// RuriV3_30m is the default bundled model: cl-nagoya/ruri-v3-30m converted to GGUF
// (the converter needs a one-line SentencePiece-vocab patch, applied host-side at
// build time) and quantized to q8_0. ModernBERT-Ja, 256-dim, mean pooling.
//
// TODO(track-b): URL is a placeholder until the converted q8_0 GGUF is hosted at an
// immutable location (e.g. a pinned Hugging Face revision). SHA256/SizeBytes are the
// spike's q8_0 artifact (verified locally); re-confirm after upload.
var RuriV3_30m = ModelSpec{
	ModelID:        "ruri-v3-30m",
	FileName:       "ruri-v3-30m-q8_0.gguf",
	URL:            "https://huggingface.co/REPLACE_OWNER/ruri-v3-30m-GGUF/resolve/REPLACE_REVISION/ruri-v3-30m-q8_0.gguf",
	SHA256:         "517044b3d5837e90e0a2b26c1ff369ee9a7985d974a5d9e9f8c9ff2cdc27d06b",
	SizeBytes:      41569120,
	Dim:            256,
	QueryPrefix:    "検索クエリ: ",
	DocumentPrefix: "検索文書: ",
}
