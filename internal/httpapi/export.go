// export.go serves the markdown transcript of a chat. Generation lives in the
// service layer (service.BuildChatMarkdown), so this handler only gathers the
// rows it needs and hands the text back.
package httpapi

import (
	"net/http"
	"time"

	"snzstudio/internal/service"
)

// handleExportChatMarkdown renders one chat as markdown.
//
// The route is chat-scoped rather than multi-agent-scoped, and serves both chat
// kinds: chats are one table discriminated by `kind`, so restricting the route
// would leave the single-assistant chats with no way out for no gain. The
// multi-agent-only sections are dropped by the builder.
//
// Participants are read with ListAll rather than ListRoster because a past
// message can name a participant that has since left the roster (design §3).
//
// The response body is the markdown itself. It carries no Content-Disposition:
// the only caller fetches it with the auth header and names the file from the
// chat title it already holds, and a filename header would have to carry
// non-ASCII titles through RFC 5987 for no reader.
func (s *Server) handleExportChatMarkdown(w http.ResponseWriter, r *http.Request) {
	chat, err := s.chats.GetChat(r.PathValue("chatId"))
	if err != nil {
		fail(w, err)
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, "chat not found")
		return
	}
	project, err := s.projects.GetProject(chat.ProjectID)
	if err != nil {
		fail(w, err)
		return
	}
	participants, err := s.participants.ListAll(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}
	messages, err := s.chats.ListMessages(chat.ID)
	if err != nil {
		fail(w, err)
		return
	}

	markdown := service.BuildChatMarkdown(project, chat, participants, messages, time.Now())
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(markdown))
}
