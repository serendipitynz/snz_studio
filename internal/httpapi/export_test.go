package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getMarkdown(t *testing.T, h http.Handler, chatID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/chats/"+chatID+"/export/markdown", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The HTTP half of AC #1-#4: a real multi-agent chat, one turn taken by a
// participant that is then removed from the roster, and a human intervention.
// The exported transcript still names all three speakers.
func TestExportChatMarkdownMultiAgent(t *testing.T) {
	llm := newMultiAgentLLM(t, nil)
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Export Project")
	chatID := createMultiAgentChat(t, h, projectID, "round_robin")
	alice := addParticipant(t, h, chatID, "Alice", llm.URL+"/v1")
	addParticipant(t, h, chatID, "Bob", llm.URL+"/v1")

	wantStatus(t, doJSON(t, h, "PATCH", "/api/chats/"+chatID,
		map[string]any{"scenePrompt": "静かな会議室"}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/turns/stream", map[string]any{}), http.StatusOK)
	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages",
		map[string]any{"content": "人間からの割り込み"}), http.StatusCreated)
	// Alice has spoken; removing her leaves her turn attributed to a participant
	// that is no longer on the roster, which is the case AC #3 names.
	wantStatus(t, doJSON(t, h, "DELETE", "/api/participants/"+alice, nil), http.StatusOK)

	rec := getMarkdown(t, h, chatID)
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("content-type = %q, want text/markdown", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"# Debate",
		"- プロジェクト: Export Project",
		"## 場面設定",
		"静かな会議室",
		"## ターン進行ルール",
		"round_robin",
		"## 編成",
		"Alice（除籍済み）",
		"Bob",
		"## 会話",
		"### Alice（除籍済み）",
		"発言します",
		"### ユーザー",
		"人間からの割り込み",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("export is missing %q\n--- body ---\n%s", want, body)
		}
	}
}

// A single-assistant chat exports through the same route; only the
// multi-agent-only sections drop out.
func TestExportChatMarkdownAssistantChat(t *testing.T) {
	h := newTestServer(t).Handler()
	projectID := createProject(t, h, "Export Project")
	chatID := createChat(t, h, projectID)

	wantStatus(t, doJSON(t, h, "POST", "/api/chats/"+chatID+"/messages",
		map[string]any{"content": "ひとこと"}), http.StatusCreated)

	rec := getMarkdown(t, h, chatID)
	wantStatus(t, rec, http.StatusOK)

	body := rec.Body.String()
	if !strings.Contains(body, "### ユーザー\n\nひとこと") {
		t.Errorf("export is missing the user's message\n--- body ---\n%s", body)
	}
	for _, unwanted := range []string{"## 場面設定", "## ターン進行ルール", "## 編成"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("assistant chat export should not carry %q\n--- body ---\n%s", unwanted, body)
		}
	}
}

func TestExportChatMarkdownUnknownChat(t *testing.T) {
	h := newTestServer(t).Handler()
	wantStatus(t, getMarkdown(t, h, "no-such-chat"), http.StatusNotFound)
}
