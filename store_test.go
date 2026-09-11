package main

import (
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAddAndGet(t *testing.T) {
	s := newTestStore(t)
	id, err := s.Add("  Buy milk  ", "HIGH", "2026-09-20")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Buy milk" || got.Priority != "high" || got.Due != "2026-09-20" || got.Done {
		t.Errorf("unexpected task: %+v", got)
	}
	if got.Created.IsZero() {
		t.Error("created timestamp not set")
	}
}

func TestValidation(t *testing.T) {
	s := newTestStore(t)
	cases := []struct {
		title, priority, due, wantErr string
	}{
		{"", "low", "", "title cannot be empty"},
		{"x", "urgent", "", "priority must be"},
		{"x", "low", "next week", "due date must look like"},
		{"x", "low", "2026-13-40", "due date must look like"},
	}
	for _, c := range cases {
		_, err := s.Add(c.title, c.priority, c.due)
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("Add(%q,%q,%q): got %v, want error containing %q", c.title, c.priority, c.due, err, c.wantErr)
		}
	}
}

func TestListFilterAndSort(t *testing.T) {
	s := newTestStore(t)
	s.Add("no due", "low", "")
	s.Add("soon", "high", "2026-01-02")
	s.Add("later", "medium", "2026-06-01")
	done, _ := s.Add("finished", "high", "")
	s.SetDone(done, true)

	titles := func(f Filter) string {
		tasks, err := s.List(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, task := range tasks {
			out = append(out, task.Title)
		}
		return strings.Join(out, ",")
	}

	if got := titles(Filter{}); got != "no due,soon,later" {
		t.Errorf("default list = %q", got)
	}
	if got := titles(Filter{All: true}); got != "no due,soon,later,finished" {
		t.Errorf("all = %q", got)
	}
	if got := titles(Filter{DoneOnly: true}); got != "finished" {
		t.Errorf("done only = %q", got)
	}
	if got := titles(Filter{Priority: "high", All: true}); got != "soon,finished" {
		t.Errorf("priority=high = %q", got)
	}
	if got := titles(Filter{Sort: "due"}); got != "soon,later,no due" {
		t.Errorf("sort=due = %q", got)
	}
	if got := titles(Filter{Sort: "priority"}); got != "soon,later,no due" {
		t.Errorf("sort=priority = %q", got)
	}
	if _, err := s.List(Filter{Sort: "title"}); err == nil {
		t.Error("expected error for unknown sort")
	}
}

func TestUpdateDoneDelete(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.Add("draft", "low", "2026-03-01")

	newTitle, clear := "final", ""
	if err := s.Update(id, &newTitle, nil, &clear); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(id)
	if got.Title != "final" || got.Priority != "low" || got.Due != "" {
		t.Errorf("after update: %+v", got)
	}
	if err := s.Update(id, nil, nil, nil); err == nil {
		t.Error("expected error when nothing to update")
	}

	if err := s.SetDone(id, true); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Get(id); !got.Done {
		t.Error("task should be done")
	}

	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(id); err == nil {
		t.Error("deleting a missing task should fail")
	}
	if _, err := s.Get(id); err == nil {
		t.Error("getting a deleted task should fail")
	}
}

func TestOverdue(t *testing.T) {
	today := "2026-09-11"
	cases := []struct {
		task Task
		want bool
	}{
		{Task{Due: "2026-09-10"}, true},
		{Task{Due: "2026-09-11"}, false},
		{Task{Due: "2026-09-12"}, false},
		{Task{Due: ""}, false},
		{Task{Due: "2026-09-10", Done: true}, false},
	}
	for _, c := range cases {
		if got := c.task.Overdue(today); got != c.want {
			t.Errorf("Overdue(%+v) = %v, want %v", c.task, got, c.want)
		}
	}
}
