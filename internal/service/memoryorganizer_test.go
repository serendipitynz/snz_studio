package service

import (
	"testing"

	"snzstudio/internal/repository"
)

func TestMemoryOrganizerFallbackDedupe(t *testing.T) {
	srv := failingLLMServer(t)
	g := newServiceGraph(t, srv.URL)

	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Two identical memories: the fallback plan should remove the duplicate.
	for i := 0; i < 2; i++ {
		if _, err := g.memories.CreateMemory(repository.CreateMemoryInput{
			ProjectID: project.ID,
			Kind:      "semantic",
			Title:     "舞台設定",
			Content:   "物語の舞台は浮遊大陸である。",
		}); err != nil {
			t.Fatalf("CreateMemory %d: %v", i, err)
		}
	}

	plan, err := g.organizer.AnalyzeProject(project.ID)
	if err != nil {
		t.Fatalf("AnalyzeProject: %v", err)
	}
	removeCount := 0
	for _, change := range plan.Changes {
		if change.Action == "remove" {
			removeCount++
			if change.MemoryID == "" || change.Reason == "" {
				t.Fatalf("remove change missing fields: %+v", change)
			}
		}
	}
	if removeCount != 1 {
		t.Fatalf("expected exactly 1 remove change, got %d (plan: %+v)", removeCount, plan)
	}

	remaining, err := g.organizer.ApplyProjectPlan(project.ID, plan)
	if err != nil {
		t.Fatalf("ApplyProjectPlan: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 memory after dedupe, got %d", len(remaining))
	}
}

func TestMemoryOrganizerLockedDuplicateKept(t *testing.T) {
	srv := failingLLMServer(t)
	g := newServiceGraph(t, srv.URL)

	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Two identical *locked* memories. The fallback dedupe must never propose
	// removing a locked memory, so no remove change should be produced (the
	// `if memory.locked { continue }` guard).
	for i := 0; i < 2; i++ {
		if _, err := g.memories.CreateMemory(repository.CreateMemoryInput{
			ProjectID: project.ID, Kind: "semantic", Title: "設定", Content: "同じ内容です。", Locked: true,
		}); err != nil {
			t.Fatalf("CreateMemory locked %d: %v", i, err)
		}
	}

	plan, err := g.organizer.AnalyzeProject(project.ID)
	if err != nil {
		t.Fatalf("AnalyzeProject: %v", err)
	}
	for _, change := range plan.Changes {
		if change.Action == "remove" {
			t.Fatalf("locked duplicate must not be removed, got change %+v", change)
		}
	}
}
