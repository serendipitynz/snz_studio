import styled from "@emotion/styled";
import { DragEvent, ReactNode, useEffect, useRef, useState } from "react";
import { useLanguage } from "../i18n";
import { snzTokens } from "../styles/themes/snz-tokens";
import { ActionButton } from "./ActionButton";
import { announce } from "./announce";
import { FileIcon, PlusIcon } from "./icons";
import { Progress } from "./Progress";

export interface DropZoneProgress {
  label: string;
  done: number;
  total: number;
}

interface FileDropZoneProps {
  // What can be dropped, the words shown while nothing is over the zone.
  label: string;
  // The accepted kinds (and any limit), in words.
  acceptWords: string;
  accepts: (file: File) => boolean;
  // A MIME type known while dragging: false only when it certainly is not accepted.
  acceptsType?: (mime: string) => boolean;
  inputAccept: string;
  multiple?: boolean;
  chooseLabel: string;
  chooseIcon?: ReactNode;
  autoFocusChoose?: boolean;
  progress?: DropZoneProgress | null;
  // Given, the zone takes nothing and says why (snz-design doc-8 §5.4).
  disabledReason?: string;
  // Every accepted file, dropped or chosen alike, so the owner writes one import path.
  // Confirming an overwrite, importing and telling the result stay with the owner.
  onFilesChosen: (files: File[]) => void;
  children?: ReactNode;
}

type ZoneState = "idle" | "over" | "reject";

// The drop zone of snz-design doc-9 §6.13. The only error it tells itself is a
// kind it does not accept; everything else fails outside it and belongs to the owner.
export function FileDropZone(props: FileDropZoneProps) {
  const { t } = useLanguage();
  const inputRef = useRef<HTMLInputElement | null>(null);
  // Entering and leaving the zone's own children fires the events again, so they are counted.
  const overCount = useRef(0);
  const [zoneState, setZoneState] = useState<ZoneState>("idle");
  const [rejected, setRejected] = useState("");
  const busy = Boolean(props.progress);
  const progressLabel = props.progress?.label;

  useEffect(() => {
    if (progressLabel) {
      announce(progressLabel);
    }
  }, [progressLabel]);

  function receive(files: File[]) {
    const misfits = files.filter((file) => !props.accepts(file));
    const fits = files.filter((file) => props.accepts(file));
    if (misfits.length > 0) {
      setRejected(
        t("dropZone.rejected", { names: misfits.map((file) => file.name).join(t("dropZone.nameSeparator")), accept: props.acceptWords })
      );
    } else {
      setRejected("");
    }
    if (fits.length > 0) {
      props.onFilesChosen(fits);
    }
  }

  function handleDragEnter(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    if (busy || props.disabledReason) {
      return;
    }
    overCount.current += 1;
    const items = Array.from(event.dataTransfer.items).filter((item) => item.kind === "file");
    const known = items.length > 0 && items.every((item) => item.type);
    const refused = known && props.acceptsType && items.every((item) => !props.acceptsType?.(item.type));
    setZoneState(refused ? "reject" : "over");
  }

  function handleDragOver(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
  }

  function handleDragLeave() {
    if (busy || props.disabledReason) {
      return;
    }
    overCount.current = Math.max(0, overCount.current - 1);
    if (overCount.current === 0) {
      setZoneState("idle");
    }
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    overCount.current = 0;
    setZoneState("idle");
    if (props.disabledReason) {
      announce(props.disabledReason);
      return;
    }
    if (busy) {
      announce(t("dropZone.busy"));
      return;
    }
    receive(Array.from(event.dataTransfer.files));
  }

  const words = zoneState === "over" ? t("dropZone.release") : zoneState === "reject" ? t("dropZone.rejectOver") : props.label;

  return (
    <Zone
      data-state={zoneState}
      data-disabled={props.disabledReason ? "" : undefined}
      aria-busy={busy || undefined}
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <FileIcon size={20} />
      <ZoneWords>{words}</ZoneWords>
      {/* The hint and the progress share one cell, so the zone keeps its size
          while it is busy and nothing below it moves (doc-9 §5.6). */}
      <SharedCell>
        <ZoneHint data-shown={!busy || undefined}>{props.acceptWords}</ZoneHint>
        {/* At rest a blank progress holds the space the real one will take. */}
        <div data-shown={busy || undefined}>
          {/* Two lines for the words, so a file name that wraps does not grow
              the zone once the import starts. */}
          <Progress
            wordLines={2}
            label={props.progress?.label ?? "\u00a0"}
            done={props.progress?.done ?? 0}
            total={props.progress?.total ?? 1}
            readout={props.progress ? t("dropZone.progress", { done: props.progress.done, total: props.progress.total }) : "\u00a0"}
          />
        </div>
      </SharedCell>
      {props.children}
      <input
        ref={inputRef}
        type="file"
        hidden
        multiple={props.multiple}
        accept={props.inputAccept}
        onChange={(event) => {
          receive(Array.from(event.target.files ?? []));
          event.target.value = "";
        }}
      />
      <ActionButton
        type="button"
        variant="normal"
        icon={props.chooseIcon ?? <PlusIcon />}
        busy={busy}
        title={busy ? t("dropZone.busy") : undefined}
        disabledReason={props.disabledReason}
        autoFocus={props.autoFocusChoose}
        onClick={() => inputRef.current?.click()}
      >
        {props.chooseLabel}
      </ActionButton>
      {rejected ? (
        <ZoneError role="alert">
          <svg
            viewBox="0 0 24 24"
            width={snzTokens.icon.sizeSm}
            height={snzTokens.icon.sizeSm}
            fill="none"
            role="img"
            aria-label={t("notice.failure")}
          >
            <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth={snzTokens.icon.strokeWidth} />
            <path d="M9 9l6 6M15 9l-6 6" stroke="currentColor" strokeWidth={snzTokens.icon.strokeWidth} strokeLinecap="round" />
          </svg>
          <span>{rejected}</span>
        </ZoneError>
      ) : null}
    </Zone>
  );
}

// The outline at rest is dashed and exempt from 3:1, as the words say where the zone
// is; the solid accent outline with a file over it tells a state, so it takes 3:1.
const Zone = styled.div`
  display: grid;
  justify-items: center;
  gap: ${snzTokens.space.sm};
  padding: ${snzTokens.space.lg} ${snzTokens.space.md};
  border: ${snzTokens.border.line} dashed ${({ theme }) => theme.dropzoneBorder};
  border-radius: ${({ theme }) => theme.radius};
  background: ${({ theme }) => theme.surfaceDropzone};
  color: ${({ theme }) => theme.muted};
  text-align: center;
  transition:
    background ${snzTokens.motion.state}ms,
    border-color ${snzTokens.motion.state}ms;

  & > svg {
    color: ${({ theme }) => theme.figure};
  }

  &[data-state="over"] {
    border-style: solid;
    border-color: ${({ theme }) => theme.accent};
    background: ${({ theme }) => theme.accentSoft};
  }

  &[data-state="over"],
  &[data-state="over"] > p,
  &[data-state="over"] > svg {
    color: ${({ theme }) => theme.onAccentSoft};
  }

  &[data-state="reject"] {
    border-style: solid;
    border-color: ${({ theme }) => theme.danger};
  }

  /* The button draws its own disabled look; the rest of the zone fades with it. */
  &[data-disabled] > :not(button) {
    opacity: ${snzTokens.opacity.disabled};
  }
`;

const ZoneWords = styled.p`
  margin: 0;
  color: ${({ theme }) => theme.ink};
  overflow-wrap: anywhere;
`;

const SharedCell = styled.div`
  display: grid;
  justify-items: center;
  inline-size: 100%;

  & > * {
    grid-area: 1 / 1;
    inline-size: 100%;
    display: grid;
    justify-items: center;
  }

  /* visibility rather than display, so the hidden one still holds its size; it
     also takes the hidden one out of what is read out. */
  & > :not([data-shown]) {
    visibility: hidden;
  }
`;

const ZoneHint = styled.p`
  margin: 0;
  font-size: ${snzTokens.font.sizeSmall};
  overflow-wrap: anywhere;
`;

const ZoneError = styled.p`
  margin: 0;
  display: flex;
  align-items: flex-start;
  gap: ${snzTokens.space.xs};
  color: ${({ theme }) => theme.dangerText};
  text-align: start;
  overflow-wrap: anywhere;

  & > svg {
    flex: none;
    margin-top: calc((1lh - ${snzTokens.icon.sizeSm}) / 2);
  }
`;
