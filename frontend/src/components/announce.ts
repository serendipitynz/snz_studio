// One polite live region for the whole app, for words that answer a press rather
// than sit on screen: the reason a disabled control gives (snz-design doc-8 §5.4)
// or why a busy drop zone takes nothing. It is created on first use and outside
// React so a control can announce from an event handler without owning a region.
let region: HTMLElement | null = null;

export function announce(words: string) {
  if (!region || !region.isConnected) {
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
    document.body.append(region);
  }
  const target = region;
  // Emptied first: the same reason pressed twice would otherwise be no change to read.
  target.textContent = "";
  window.setTimeout(() => {
    target.textContent = words;
  }, 50);
}
