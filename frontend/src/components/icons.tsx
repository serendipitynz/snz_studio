import { keyframes } from "@emotion/react";
import styled from "@emotion/styled";
import type { ReactNode } from "react";
import { snzTokens } from "../styles/themes/snz-tokens";

// Figures copied from Lucide 1.48.0 (the chat screens' figures from panel-right-open
// down to download come from 1.47.0; npm's minimum release age held 1.48.0 back) (https://lucide.dev), path data unchanged, under
// the ISC licence in icons.LICENSE (trash and lock come from Feather, MIT, same file).
// Copied rather than installed as lucide-react to keep the dependency list as it is.
// Hidden from assistive technology: every button showing one also carries words or
// an aria-label.
function Lucide({ size = 16, children }: { size?: number; children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ display: "block", flexShrink: 0 }}
    >
      {children}
    </svg>
  );
}

export function HouseIcon({ size }: { size?: number }) {
  return (
    <Lucide size={size}>
      <path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8" />
      <path d="M3 10a2 2 0 0 1 .709-1.528l7-6a2 2 0 0 1 2.582 0l7 6A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    </Lucide>
  );
}

export function SettingsIcon({ size }: { size?: number }) {
  return (
    <Lucide size={size}>
      <path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915" />
      <circle cx="12" cy="12" r="3" />
    </Lucide>
  );
}

export function MessageSquarePlusIcon() {
  return (
    <Lucide>
      <path d="M22 17a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 21.286V5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2z" />
      <path d="M12 8v6" />
      <path d="M9 11h6" />
    </Lucide>
  );
}

export function PencilIcon() {
  return (
    <Lucide>
      <path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" />
      <path d="m15 5 4 4" />
    </Lucide>
  );
}

export function ImageIcon() {
  return (
    <Lucide>
      <rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
      <circle cx="9" cy="9" r="2" />
      <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
    </Lucide>
  );
}

export function TrashIcon() {
  return (
    <Lucide>
      <path d="M10 11v6" />
      <path d="M14 11v6" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
      <path d="M3 6h18" />
      <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    </Lucide>
  );
}

export function FilePlusIcon() {
  return (
    <Lucide>
      <path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z" />
      <path d="M14 2v5a1 1 0 0 0 1 1h5" />
      <path d="M9 15h6" />
      <path d="M12 18v-6" />
    </Lucide>
  );
}

export function FileIcon({ size }: { size?: number }) {
  return (
    <Lucide size={size}>
      <path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z" />
      <path d="M14 2v5a1 1 0 0 0 1 1h5" />
    </Lucide>
  );
}

export function LockIcon() {
  return (
    <Lucide>
      <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </Lucide>
  );
}

export function LockOpenIcon() {
  return (
    <Lucide>
      <rect width="18" height="11" x="3" y="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 9.9-1" />
    </Lucide>
  );
}

// Shared with every participant: several figures.
export function UsersIcon() {
  return (
    <Lucide>
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <path d="M16 3.128a4 4 0 0 1 0 7.744" />
      <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
      <circle cx="9" cy="7" r="4" />
    </Lucide>
  );
}

// Kept to the speakers that receive the project material: one figure with a shield.
export function UserShieldIcon() {
  return (
    <Lucide>
      <path d="M10 15H6a4 4 0 0 0-4 4v2" />
      <path d="M22 17.5c0 2.499-1.75 3.749-3.83 4.474a.5.5 0 0 1-.335-.005c-2.085-.72-3.835-1.97-3.835-4.47V14a.5.5 0 0 1 .5-.499c1 0 2.25-.6 3.12-1.36a.6.6 0 0 1 .76-.001c.875.765 2.12 1.36 3.12 1.36a.5.5 0 0 1 .5.5z" />
      <circle cx="9" cy="7" r="4" />
    </Lucide>
  );
}

export function FolderIcon({ size }: { size?: number }) {
  return (
    <Lucide size={size}>
      <path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z" />
    </Lucide>
  );
}

export function PlusIcon() {
  return (
    <Lucide>
      <path d="M5 12h14" />
      <path d="M12 5v14" />
    </Lucide>
  );
}

export function CheckIcon() {
  return (
    <Lucide>
      <path d="M20 6 9 17l-5-5" />
    </Lucide>
  );
}

export function SparklesIcon() {
  return (
    <Lucide>
      <path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z" />
      <path d="M20 2v4" />
      <path d="M22 4h-4" />
      <circle cx="4" cy="20" r="2" />
    </Lucide>
  );
}

export function SlidersIcon() {
  return (
    <Lucide>
      <path d="M10 5H3" />
      <path d="M12 19H3" />
      <path d="M14 3v4" />
      <path d="M16 17v4" />
      <path d="M21 12h-9" />
      <path d="M21 19h-5" />
      <path d="M21 5h-7" />
      <path d="M8 10v4" />
      <path d="M8 12H3" />
    </Lucide>
  );
}

// Shows a side region that is hidden (snz-design doc-9 §6.3.1): the arrow points
// the way the region will come in.
export function PanelRightOpenIcon() {
  return (
    <Lucide>
      <rect width="18" height="18" x="3" y="3" rx="2" />
      <path d="M15 3v18" />
      <path d="m10 15-3-3 3-3" />
    </Lucide>
  );
}

export function PanelRightCloseIcon() {
  return (
    <Lucide>
      <rect width="18" height="18" x="3" y="3" rx="2" />
      <path d="M15 3v18" />
      <path d="m8 9 3 3-3 3" />
    </Lucide>
  );
}

// Turned a quarter while its section is open (snz-design doc-9 §6.3).
export function ChevronRightIcon() {
  return (
    <Lucide>
      <path d="m9 18 6-6-6-6" />
    </Lucide>
  );
}

export function CircleHelpIcon() {
  return (
    <Lucide>
      <circle cx="12" cy="12" r="10" />
      <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
      <path d="M12 17h.01" />
    </Lucide>
  );
}

export function MoveUpIcon() {
  return (
    <Lucide>
      <path d="M8 6L12 2L16 6" />
      <path d="M12 2V22" />
    </Lucide>
  );
}

export function MoveDownIcon() {
  return (
    <Lucide>
      <path d="M8 18L12 22L16 18" />
      <path d="M12 2V22" />
    </Lucide>
  );
}

export function ClipboardIcon() {
  return (
    <Lucide>
      <rect width="8" height="4" x="8" y="2" rx="1" ry="1" />
      <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
    </Lucide>
  );
}

// The copy button's figure for a moment after a copy.
export function ClipboardCheckIcon() {
  return (
    <Lucide>
      <rect width="8" height="4" x="8" y="2" rx="1" ry="1" />
      <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
      <path d="m9 14 2 2 4-4" />
    </Lucide>
  );
}

// The editorial review of an answer.
export function MessageSquareCodeIcon() {
  return (
    <Lucide>
      <path d="M22 17a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 21.286V5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2z" />
      <path d="m10 8-3 3 3 3" />
      <path d="m14 14 3-3-3-3" />
    </Lucide>
  );
}

// Saving an utterance as a memory.
export function MemoryStickIcon() {
  return (
    <Lucide>
      <path d="M12 12v-2" />
      <path d="M12 18v-2" />
      <path d="M16 12v-2" />
      <path d="M16 18v-2" />
      <path d="M2 11h1.5" />
      <path d="M20 18v-2" />
      <path d="M20.5 11H22" />
      <path d="M4 18v-2" />
      <path d="M8 12v-2" />
      <path d="M8 18v-2" />
      <rect x="2" y="6" width="20" height="10" rx="2" />
    </Lucide>
  );
}

// Drafting the conversation's conclusion.
export function SummaryIcon() {
  return (
    <Lucide>
      <path d="M15 4H7" />
      <path d="m18 16 3 3-3 3" />
      <path d="M3 4v13a2 2 0 0 0 2 2h16" />
      <path d="M7 14h7" />
      <path d="M7 9h12" />
    </Lucide>
  );
}

// Reading the conversation again from the server.
export function RotateCwIcon() {
  return (
    <Lucide>
      <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8" />
      <path d="M21 3v5h-5" />
    </Lucide>
  );
}

export function MessagesSquareIcon({ size }: { size?: number }) {
  return (
    <Lucide size={size}>
      <path d="M16 10a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 14.286V4a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" />
      <path d="M20 9a2 2 0 0 1 2 2v10.286a.71.71 0 0 1-1.212.502l-2.202-2.202A2 2 0 0 0 17.172 19H10a2 2 0 0 1-2-2v-1" />
    </Lucide>
  );
}

export function SendIcon() {
  return (
    <Lucide>
      <path d="M3.714 3.048a.498.498 0 0 0-.683.627l2.843 7.627a2 2 0 0 1 0 1.396l-2.842 7.627a.498.498 0 0 0 .682.627l18-8.5a.5.5 0 0 0 0-.904z" />
      <path d="M6 12h16" />
    </Lucide>
  );
}

export function StepForwardIcon() {
  return (
    <Lucide>
      <path d="M10.029 4.285A2 2 0 0 0 7 6v12a2 2 0 0 0 3.029 1.715l9.997-5.998a2 2 0 0 0 .003-3.432z" />
      <path d="M3 4v16" />
    </Lucide>
  );
}

export function PlayIcon() {
  return (
    <Lucide>
      <path d="M5 5a2 2 0 0 1 3.008-1.728l11.997 6.998a2 2 0 0 1 .003 3.458l-12 7A2 2 0 0 1 5 19z" />
    </Lucide>
  );
}

export function SquareIcon() {
  return (
    <Lucide>
      <rect width="18" height="18" x="3" y="3" rx="2" />
    </Lucide>
  );
}

export function PlugIcon() {
  return (
    <Lucide>
      <path d="M12 22v-5" />
      <path d="M15 8V2" />
      <path d="M17 8a1 1 0 0 1 1 1v4a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V9a1 1 0 0 1 1-1z" />
      <path d="M9 8V2" />
    </Lucide>
  );
}

export function UserPlusIcon() {
  return (
    <Lucide>
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <line x1="19" x2="19" y1="8" y2="14" />
      <line x1="22" x2="16" y1="11" y2="11" />
    </Lucide>
  );
}

export function DownloadIcon() {
  return (
    <Lucide>
      <path d="M12 15V3" />
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <path d="m7 10 5 5 5-5" />
    </Lucide>
  );
}

export function XIcon() {
  return (
    <Lucide>
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </Lucide>
  );
}

const spin = keyframes`
  to {
    transform: rotate(1turn);
  }
`;

// Rotated by CSS rather than SMIL, which cannot read the reduced-motion setting.
// Reduced motion slows it rather than stopping it: a still figure reads as idle
// (snz-design doc-8 §6.7).
const Spinner = styled.svg`
  display: block;
  flex-shrink: 0;
  animation: ${spin} ${snzTokens.motion.spin}ms linear infinite;

  @media (prefers-reduced-motion: reduce) {
    animation-duration: ${snzTokens.motion.spinReduced}ms;
  }
`;

// Not a Lucide figure: the app's busy figure, kept as it is on the other screens.
export function SpinnerIcon({ size = 16 }: { size?: number }) {
  return (
    <Spinner width={size} height={size} viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <circle cx="8" cy="8" r="5.5" stroke="currentColor" strokeOpacity="0.22" strokeWidth="1.6" />
      <path d="M13.5 8A5.5 5.5 0 0 0 8 2.5" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </Spinner>
  );
}
