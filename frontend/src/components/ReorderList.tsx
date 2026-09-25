import styled from "@emotion/styled";
import {
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  ReactNode,
  useEffect,
  useLayoutEffect,
  useRef,
  useState
} from "react";
import { useLanguage } from "../i18n";
import { snzTokens } from "../styles/themes/snz-tokens";
import { focusRing, VisuallyHidden } from "../styles/ui";

// Movement under this many pixels is a press, not a drag: a hand resting on a
// mouse jitters, and a press is what leaves the item grabbed for the
// single-pointer path.
const DRAG_THRESHOLD = 4;

interface Grab {
  id: string;
  from: number;
  to: number;
}

interface ReorderListProps<T extends { id: string }> {
  items: T[];
  label: string;
  nameOf: (item: T) => string;
  renderItem: (item: T) => ReactNode;
  // Saving is the caller's (snz-design doc-9 §5.5); the list only reports
  // where the item was placed.
  onReordered: (id: string, toIndex: number) => void;
  busy?: boolean;
}

// snz-design doc-9 §6.9. Dragging the handle is one way to move an item; the
// same handle also takes the two alternatives the spec requires: press it once
// and press a drop position (single pointer, 2.5.7), or Space, the arrows and
// Space (keyboard). Pressing the grabbed handle again or Escape cancels.
// Drag and drop is built on pointer events rather than the HTML drag API,
// because the single-pointer path needs the handle to stay grabbed after a
// plain press, which a drag source cannot do.
export function ReorderList<T extends { id: string }>(props: ReorderListProps<T>) {
  const { items, nameOf, onReordered } = props;
  const { t } = useLanguage();
  const [grab, setGrab] = useState<Grab | null>(null);
  const [dragOffset, setDragOffset] = useState<number | null>(null);
  const [announcement, setAnnouncement] = useState("");
  // The document listeners of a drag read the grab from here, since they
  // outlive the render that attached them.
  const grabRef = useRef<Grab | null>(null);
  const handleRefs = useRef(new Map<string, HTMLButtonElement>());
  const rowRefs = useRef(new Map<string, HTMLLIElement>());
  const stopDragRef = useRef<(() => void) | null>(null);
  const focusAfterRender = useRef<string | null>(null);

  const count = items.length;
  const nameById = (id: string) => {
    const item = items.find((candidate) => candidate.id === id);
    return item ? nameOf(item) : "";
  };

  function update(next: Grab | null) {
    grabRef.current = next;
    setGrab(next);
  }

  // Keyed rows are moved in the DOM when the order changes, which can drop
  // the focus from the moved handle, so it is put back once the rows settle.
  useLayoutEffect(() => {
    const id = focusAfterRender.current;
    if (id) {
      focusAfterRender.current = null;
      handleRefs.current.get(id)?.focus();
    }
  });

  // The grabbed item can vanish under the list (the caller reloaded it).
  useEffect(() => {
    if (grab && !items.some((item) => item.id === grab.id)) {
      stopDragRef.current?.();
      update(null);
      setDragOffset(null);
    }
  }, [items, grab]);

  useEffect(() => () => stopDragRef.current?.(), []);

  function startGrab(id: string) {
    const current = grabRef.current;
    if (current?.id === id) {
      cancel();
      return;
    }
    const from = items.findIndex((item) => item.id === id);
    update({ id, from, to: from });
    setAnnouncement(t("reorder.grabbed", { name: nameById(id), position: from + 1, count }));
  }

  function place() {
    const current = grabRef.current;
    if (!current) {
      return;
    }
    stopDragRef.current?.();
    update(null);
    setDragOffset(null);
    focusAfterRender.current = current.id;
    if (current.to === current.from) {
      return;
    }
    setAnnouncement(t("reorder.placed", { name: nameById(current.id), position: current.to + 1, count }));
    onReordered(current.id, current.to);
  }

  function cancel() {
    const current = grabRef.current;
    if (!current) {
      return;
    }
    stopDragRef.current?.();
    update(null);
    setDragOffset(null);
    focusAfterRender.current = current.id;
    setAnnouncement(t("reorder.cancelled", { name: nameById(current.id), position: current.from + 1, count }));
  }

  function moveTo(to: number) {
    const current = grabRef.current;
    if (!current || to === current.to) {
      return;
    }
    update({ ...current, to });
  }

  // Escape reaches the list wherever the focus is while an item is grabbed:
  // after a single-pointer grab it can sit on a drop position.
  useEffect(() => {
    if (!grab) {
      return;
    }
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape" && !event.isComposing) {
        event.preventDefault();
        event.stopPropagation();
        cancel();
      }
    }
    document.addEventListener("keydown", handleKeyDown, true);
    return () => document.removeEventListener("keydown", handleKeyDown, true);
  }, [grab]);

  // The final index for the pointer's height: before the first row whose
  // middle lies below it, among the rows other than the grabbed one.
  function indexAt(clientY: number, current: Grab) {
    let gap = count;
    for (const [index, item] of items.entries()) {
      if (item.id === current.id) {
        continue;
      }
      const box = rowRefs.current.get(item.id)?.getBoundingClientRect();
      if (box && clientY < box.top + box.height / 2) {
        gap = index;
        break;
      }
    }
    return gap > current.from ? gap - 1 : gap;
  }

  function handlePointerDown(event: ReactPointerEvent<HTMLButtonElement>, id: string) {
    if (event.button !== 0 || props.busy) {
      return;
    }
    const handle = event.currentTarget;
    startGrab(id);
    if (grabRef.current?.id !== id) {
      return;
    }

    stopDragRef.current?.();
    const pointerId = event.pointerId;
    const startY = event.clientY;
    let dragged = false;

    function handleMove(moveEvent: PointerEvent) {
      const current = grabRef.current;
      if (moveEvent.pointerId !== pointerId || current?.id !== id) {
        return;
      }
      const offset = moveEvent.clientY - startY;
      if (!dragged && Math.abs(offset) < DRAG_THRESHOLD) {
        return;
      }
      dragged = true;
      // The row following the pointer answers the drag directly, so it is
      // not an animation that reduced motion would take away (doc-9 §6.9).
      setDragOffset(offset);
      moveTo(indexAt(moveEvent.clientY, current));
    }
    function handleUp(upEvent: PointerEvent) {
      if (upEvent.pointerId !== pointerId) {
        return;
      }
      stop();
      // A press without movement leaves the item grabbed for the drop positions.
      if (dragged && grabRef.current?.id === id) {
        place();
      }
    }
    // The browser took the pointer away: the drag is abandoned, not committed.
    function handleCancel(cancelEvent: PointerEvent) {
      if (cancelEvent.pointerId !== pointerId) {
        return;
      }
      stop();
      if (grabRef.current?.id === id) {
        cancel();
      }
    }
    function stop() {
      document.removeEventListener("pointermove", handleMove);
      document.removeEventListener("pointerup", handleUp);
      document.removeEventListener("pointercancel", handleCancel);
      if (stopDragRef.current === stop) {
        stopDragRef.current = null;
      }
    }

    try {
      handle.setPointerCapture(pointerId);
    } catch {
      // No active pointer to capture; the document listeners suffice.
    }
    document.addEventListener("pointermove", handleMove);
    document.addEventListener("pointerup", handleUp);
    document.addEventListener("pointercancel", handleCancel);
    stopDragRef.current = stop;
  }

  // A click the pointer did not make (detail 0) is the button's own
  // activation: Space, Enter, or a screen reader's press, which sends no
  // pointer events. It grabs and places like Space in doc-9 §6.9, so every
  // way of pressing the handle reaches the keyboard path. A pointer's click
  // already acted on pointerdown and only settles the focus: WebKit does not
  // focus a button it is pressed on, and focusing after the press keeps it a
  // pointer focus without a ring (doc-8 §5.1), where focusing on pointerdown
  // would draw one in Chromium.
  function handleClick(event: ReactMouseEvent<HTMLButtonElement>, id: string) {
    if (event.detail !== 0) {
      if (document.activeElement !== event.currentTarget) {
        event.currentTarget.focus();
      }
      return;
    }
    if (props.busy) {
      return;
    }
    if (grabRef.current?.id === id) {
      place();
    } else {
      startGrab(id);
    }
  }

  function handleKeyDown(event: ReactKeyboardEvent<HTMLButtonElement>, id: string) {
    if (event.nativeEvent.isComposing) {
      return;
    }
    const step = event.key === "ArrowDown" ? 1 : event.key === "ArrowUp" ? -1 : 0;
    const current = grabRef.current;
    if (!step || current?.id !== id) {
      return;
    }
    event.preventDefault();
    // Stays inside the list (doc-9 §6.9, doc-8 §5.6).
    const to = Math.min(count - 1, Math.max(0, current.to + step));
    if (to === current.to) {
      return;
    }
    moveTo(to);
    setAnnouncement(t("reorder.moved", { position: to + 1, count }));
  }

  // A gap k lies before the row at k (k = count is after the last row). The
  // two gaps beside the grabbed row would leave it where it is, so they offer
  // no drop position; cancelling is the handle or Escape.
  const gapFor = (current: Grab) => (current.to < current.from ? current.to : current.to + 1);
  const isStay = (gap: number, current: Grab) => gap === current.from || gap === current.from + 1;
  const toFor = (gap: number, current: Grab) => (gap > current.from ? gap - 1 : gap);

  function dropPosition(gap: number, edge: "start" | "end", current: Grab) {
    const to = toFor(gap, current);
    return (
      <DropPosition
        key={edge}
        type="button"
        data-edge={edge}
        data-predicted={current.to !== current.from && gapFor(current) === gap ? "" : undefined}
        aria-label={t("reorder.dropAt", { position: to + 1 })}
        onClick={() => {
          moveTo(to);
          place();
        }}
      />
    );
  }

  return (
    <>
      <OrderedList aria-label={props.label} aria-busy={props.busy || undefined} data-grabbing={grab ? "" : undefined}>
        {items.map((item, index) => {
          const grabbed = grab?.id === item.id;
          return (
            <Row
              key={item.id}
              ref={(element: HTMLLIElement | null) => {
                if (element) {
                  rowRefs.current.set(item.id, element);
                } else {
                  rowRefs.current.delete(item.id);
                }
              }}
              data-grabbed={grabbed ? "" : undefined}
              data-dragging={grabbed && dragOffset !== null ? "" : undefined}
              style={grabbed && dragOffset !== null ? { translate: `0 ${dragOffset}px` } : undefined}
            >
              <Handle
                ref={(element: HTMLButtonElement | null) => {
                  if (element) {
                    handleRefs.current.set(item.id, element);
                  } else {
                    handleRefs.current.delete(item.id);
                  }
                }}
                type="button"
                aria-label={t("reorder.handle", { name: nameOf(item) })}
                aria-pressed={grabbed}
                aria-busy={props.busy || undefined}
                title={props.busy ? t("reorder.saving") : undefined}
                onPointerDown={(event) => handlePointerDown(event, item.id)}
                onClick={(event) => handleClick(event, item.id)}
                onKeyDown={(event) => handleKeyDown(event, item.id)}
              >
                <GripIcon />
              </Handle>
              {props.renderItem(item)}
              {grab && !isStay(index, grab) ? dropPosition(index, "start", grab) : null}
              {grab && index === count - 1 && !isStay(count, grab) ? dropPosition(count, "end", grab) : null}
            </Row>
          );
        })}
      </OrderedList>
      <VisuallyHidden aria-live="polite">{announcement}</VisuallyHidden>
    </>
  );
}

function GripIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M9 5v2M15 5v2M9 11v2M15 11v2M9 17v2M15 17v2"
        stroke="currentColor"
        strokeWidth={snzTokens.icon.strokeWidth + 0.5}
        strokeLinecap="round"
      />
    </svg>
  );
}

// One box of rows split by divider lines, as the shared row list draws it
// (snz-design doc-9 §6.1): the rows carry a single column, so they do not
// need to stand apart as cards.
const OrderedList = styled.ol`
  display: flex;
  flex-direction: column;
  min-width: 0;
  margin: 0;
  padding: 0;
  list-style: none;
  border: ${snzTokens.border.line} solid ${({ theme }) => theme.fieldBorder};
  border-radius: ${({ theme }) => theme.radius};
  background: ${({ theme }) => theme.surfaceItem};
`;

const Row = styled.li`
  position: relative;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: ${snzTokens.space.xs};
  padding: 0 ${snzTokens.space.xs} 0 ${snzTokens.space.sm};
  min-width: 0;
  background: ${({ theme }) => theme.surfaceItem};
  /* Row separators are decorative: no contrast minimum (doc-5 §3.2). */
  border-bottom: ${snzTokens.border.line} solid ${({ theme }) => theme.line};

  &:last-child {
    border-bottom: 0;
  }

  /* The faces follow the box's rounded corners, which is not clipped: a
     clipping box would cut the focus rings inside it. */
  &:first-child {
    border-top-left-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
    border-top-right-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
  }

  &:last-child {
    border-bottom-left-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
    border-bottom-right-radius: calc(${({ theme }) => theme.radius} - ${snzTokens.border.line});
  }

  &:not([data-grabbed]):hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  /* The link fills the rest of the row, whose neighbours touch it, so its
     ring is drawn just inside it rather than over the next row. */
  & > a:focus-visible {
    outline-offset: calc(-1 * ${snzTokens.border.focus});
  }

  /* The face, the frame and the left band together (doc-8 §5.2). */
  &[data-grabbed] {
    z-index: 1;
    background: ${({ theme }) => theme.surfaceSelected};
    outline: ${snzTokens.border.line} solid ${({ theme }) => theme.selected};
    outline-offset: calc(-1 * ${snzTokens.border.line});
    box-shadow: inset ${snzTokens.border.band} 0 0 ${({ theme }) => theme.selected};
  }

  /* While dragging, the grabbed row floats over its neighbours. */
  &[data-dragging] {
    z-index: 3;
    box-shadow:
      inset ${snzTokens.border.band} 0 0 ${({ theme }) => theme.selected},
      ${({ theme }) => theme.shadowPopover};
  }
`;

// The quiet icon-only button of the shared reorder example: no face and no
// outline at rest, only the figure (doc-8 §6.1 / §6.2). The pressable area
// stays above the 24px minimum (doc-8 §5.7).
const Handle = styled.button`
  position: relative;
  z-index: 2;
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: ${({ theme }) => theme.radiusSm};
  background: transparent;
  color: ${({ theme }) => theme.figure};
  cursor: grab;
  touch-action: none;
  user-select: none;
  -webkit-user-select: none;

  &:not([aria-busy="true"]):hover {
    background: ${({ theme }) => theme.surfaceHover};
  }

  &:not([aria-busy="true"]):active {
    background: ${({ theme }) => theme.surfacePressed};
  }

  &[aria-busy="true"] {
    cursor: default;
  }

  [data-grabbed] > & {
    cursor: grabbing;
  }

  ${focusRing}
`;

// Laid over the boundary between two rows instead of inserted between them,
// so grabbing does not move the list. The pressable band is 24px tall
// (doc-8 §5.7), centred on the 1px divider, and drawn as a thin dashed line
// that turns into the solid line of the predicted drop (doc-9 §6.9).
const DropPosition = styled.button`
  position: absolute;
  left: 0;
  right: 0;
  z-index: 1;
  height: ${snzTokens.size.targetMin};
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;

  &[data-edge="start"] {
    top: calc(-${snzTokens.size.targetMin} / 2 - ${snzTokens.border.line} / 2);
  }

  &[data-edge="end"] {
    bottom: calc(-${snzTokens.size.targetMin} / 2 - ${snzTokens.border.line} / 2);
  }

  &::before {
    content: "";
    position: absolute;
    left: ${snzTokens.space.sm};
    right: ${snzTokens.space.sm};
    top: 50%;
    border-top: 2px dashed ${({ theme }) => theme.fieldBorder};
    translate: 0 -50%;
  }

  &:hover::before,
  &[data-predicted]::before {
    border-top: calc(${snzTokens.border.band} + 1px) solid ${({ theme }) => theme.selected};
  }

  &:focus-visible {
    outline: ${snzTokens.border.focus} solid ${({ theme }) => theme.focus};
    outline-offset: calc(-1 * ${snzTokens.border.focus});
  }
`;
