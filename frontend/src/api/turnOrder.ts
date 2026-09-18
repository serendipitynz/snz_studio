import { MessageRecord, Participant, TurnRule } from "./client";

// predictNextSpeaker mirrors the turn rule the engine applies in
// internal/service/turnengine.go (docs/multi-agent-chat-design.md §4.2 step 1).
//
// The engine only reveals who spoke once the turn is stored, so the spectator
// view has nothing to label an in-flight utterance with unless it derives the
// speaker itself. It can, because the rule reads the same two things the client
// already holds: the roster's order and the last message carrying a
// participantId. A prediction that diverges (the roster changed under a running
// turn) is corrected by the `done` frame, so this steers a label, never a
// request.
//
// roster must be the in-roster participants in sort_order, which is the order the
// participants routes return.
export function predictNextSpeaker(
  roster: Participant[],
  messages: MessageRecord[],
  turnRule: TurnRule,
  nomineeId: string
): Participant | null {
  if (turnRule === "manual") {
    return roster.find((participant) => participant.id === nomineeId) ?? null;
  }
  if (roster.length === 0) {
    return null;
  }

  const lastParticipantId = [...messages].reverse().find((message) => message.participantId)?.participantId;
  if (!lastParticipantId) {
    return roster[0];
  }

  const index = roster.findIndex((participant) => participant.id === lastParticipantId);
  // The last speaker is off the roster, so the cycle has no position to advance
  // from and restarts at the head — the engine's own fallback.
  if (index < 0) {
    return roster[0];
  }
  return roster[(index + 1) % roster.length];
}
