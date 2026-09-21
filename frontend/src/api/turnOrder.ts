import { MessageRecord, Participant, TurnRule } from "./client";

// predictNextSpeaker mirrors the turn rule the engine applies in
// internal/service/turnengine.go (docs/multi-agent-chat-design.md §4.2 step 1).
//
// The engine only reveals who spoke once the turn is stored, so the spectator
// view has nothing to label an in-flight utterance with unless it derives the
// speaker itself. It can, because the rules read what the client already holds:
// the roster's order, the chat's facilitator and the stored messages. A
// prediction that diverges (the roster changed under a running turn) is
// corrected by the `done` frame, so this steers a label, never a request.
//
// roster must be the in-roster participants in sort_order, which is the order the
// participants routes return.
export function predictNextSpeaker(
  roster: Participant[],
  messages: MessageRecord[],
  turnRule: TurnRule,
  nomineeId: string,
  facilitatorId: string
): Participant | null {
  if (turnRule === "manual") {
    return roster.find((participant) => participant.id === nomineeId) ?? null;
  }
  if (roster.length === 0) {
    return null;
  }
  if (turnRule === "facilitator_alternating") {
    const facilitator = roster.find((participant) => participant.id === facilitatorId);
    // An unset facilitator and one that has left the roster are the same case,
    // and both fall back to round_robin — the engine's own degradation.
    if (facilitator) {
      return alternatingSpeaker(roster, facilitator, messages);
    }
  }
  return roundRobinSpeaker(roster, messages);
}

function roundRobinSpeaker(roster: Participant[], messages: MessageRecord[]): Participant {
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

// alternatingSpeaker is facilitator_alternating: the facilitator and the rest of
// the roster speak one utterance each in turn. The human's own interventions
// carry no participantId and round_robin looks past them; here the last message
// is read whoever spoke it, so the facilitator answers an intervention — as it
// does the opening turn.
function alternatingSpeaker(
  roster: Participant[],
  facilitator: Participant,
  messages: MessageRecord[]
): Participant {
  const others = roster.filter((participant) => participant.id !== facilitator.id);
  if (others.length === 0) {
    return facilitator;
  }
  const last = messages[messages.length - 1];
  if (!last || last.participantId !== facilitator.id) {
    return facilitator;
  }

  const previous = [...messages]
    .reverse()
    .find((message) => message.participantId && message.participantId !== facilitator.id)?.participantId;
  const index = others.findIndex((participant) => participant.id === previous);
  if (index < 0) {
    return others[0];
  }
  return others[(index + 1) % others.length];
}
