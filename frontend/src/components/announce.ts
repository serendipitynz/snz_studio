// One polite live region for the whole app, for words that answer a press rather
// than sit on screen: the reason a disabled control gives (snz-design doc-8 §5.4)
// or why a busy drop zone takes nothing. It is created on first use and outside
// React so a control can announce from an event handler without owning a region.
// It lives inside the topmost open modal when there is one: screen readers treat
// what is outside an aria-modal dialog as inert.
let region: HTMLElement | null = null;

export function announce(words: string) {
  const dialogs = document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]');
  const host = dialogs.length > 0 ? dialogs[dialogs.length - 1] : document.body;
  if (region && region.parentElement !== host) {
    region.remove();
    region = null;
  }
  if (!region) {
    region = document.createElement("div");
    region.setAttribute("aria-live", "polite");
    Object.assign(region.style, {
      position: "absolute",
      width: "1px",
      height: "1px",
      overflow: "hidden",
      clipPath: "inset(50%)",
      whiteSpace: "nowrap"
    });
    host.append(region);
  }
  const target = region;
  // Emptied first: the same reason pressed twice would otherwise be no change to read.
  target.textContent = "";
  window.setTimeout(() => {
    target.textContent = words;
  }, 50);
}
