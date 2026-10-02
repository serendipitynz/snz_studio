package service

import (
	"reflect"
	"testing"

	"snzstudio/internal/model"
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

// TestMemoryOrganizerApplyKeepsOtherProjectsAndLocked covers a plan that does not
// come from AnalyzeProject — sent straight to the API, or carrying an ID the LLM
// took from elsewhere. Only the URL project's unlocked memories may change.
func TestMemoryOrganizerApplyKeepsOtherProjectsAndLocked(t *testing.T) {
	srv := failingLLMServer(t)
	g := newServiceGraph(t, srv.URL)

	project, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Saga"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	other, err := g.projects.CreateProject(repository.CreateProjectInput{Title: "Other"})
	if err != nil {
		t.Fatalf("CreateProject other: %v", err)
	}
	create := func(projectID, title string, locked bool) model.Memory {
		t.Helper()
		m, err := g.memories.CreateMemory(repository.CreateMemoryInput{
			ProjectID: projectID, Kind: "semantic", Title: title, Content: title + "の内容", Locked: locked,
		})
		if err != nil {
			t.Fatalf("CreateMemory %s: %v", title, err)
		}
		return m
	}
	own := create(project.ID, "不要な設定", false)
	ownLocked := create(project.ID, "固定した設定", true)
	foreign := create(other.ID, "別作品の設定", false)
	foreignLocked := create(other.ID, "別作品の固定設定", true)

	untouched := []model.Memory{ownLocked, foreign, foreignLocked}
	before := map[string]model.Memory{}
	for _, m := range untouched {
		got, err := g.memories.GetMemory(m.ID)
		if err != nil || got == nil {
			t.Fatalf("GetMemory %s: %v, %v", m.ID, got, err)
		}
		before[m.ID] = *got
	}

	changes := []model.MemoryOrganizationChange{{Action: "remove", MemoryID: own.ID, Reason: "不要"}}
	for _, m := range untouched {
		changes = append(changes,
			model.MemoryOrganizationChange{Action: "update", MemoryID: m.ID, Kind: "episodic", Title: "上書き", Content: "上書きされた", Reason: "書き換え"},
			model.MemoryOrganizationChange{Action: "remove", MemoryID: m.ID, Reason: "削除"},
		)
	}

	remaining, err := g.organizer.ApplyProjectPlan(project.ID, model.MemoryOrganizationPlan{Changes: changes})
	if err != nil {
		t.Fatalf("ApplyProjectPlan: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != ownLocked.ID {
		t.Fatalf("remaining = %+v, want only the locked memory of the project", remaining)
	}
	for id, want := range before {
		got, err := g.memories.GetMemory(id)
		if err != nil || got == nil {
			t.Fatalf("memory %s gone: %v, %v", id, got, err)
		}
		if !reflect.DeepEqual(*got, want) {
			t.Errorf("memory %s changed:\n got %+v\nwant %+v", id, *got, want)
		}
	}
}
