package service

import (
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// a look at a session in the checkout reads it through its harness and
// changes nothing: no conversation, no event, no registry write. discovery
// says how many sessions it left out because a conversation holds them.
func TestALookChangesNothingAndDiscoveryCountsTheHeld(t *testing.T) {
	r := newRig(t)
	held := r.start("one")
	r.fake.SetSessions([]harness.Session{
		{NativeID: held.NativeID, Preview: "one"},
		{NativeID: "terminal-1", Preview: "Started in a terminal"},
	})
	r.fake.SetHistory("terminal-1", []*model.Event{
		{Kind: model.EventUser, Text: "Started in a terminal", NativeID: "t-1"},
		{Kind: model.EventAssistant, Text: "a reply", NativeID: "t-2"},
		{Kind: model.EventUser, Text: "and then this", NativeID: "t-3"},
		{Kind: model.EventAssistant, Text: "the last reply", NativeID: "t-4"},
	})
	found, err := r.svc.DiscoverHeld(r.project.ID, model.HarnessClaude)
	if err != nil || len(found.Sessions) != 1 || found.Sessions[0].NativeID != "terminal-1" || found.Held != 1 {
		t.Fatalf("discovered %+v, %v", found, err)
	}
	before := workspaceOf(t, r.svc)
	look, err := r.svc.Look(r.project.ID, model.HarnessClaude, "terminal-1")
	if err != nil {
		t.Fatal(err)
	}
	if look.FirstPrompt != "Started in a terminal" || look.LastPrompt != "and then this" || look.LastReply != "the last reply" || look.Prompts != 2 {
		t.Errorf("look: %+v", look)
	}
	after := workspaceOf(t, r.svc)
	if len(after.Conversations) != len(before.Conversations) || len(after.NeedsYou) != len(before.NeedsYou) {
		t.Errorf("the look changed the workspace: %d conversations, %d waiting; were %d, %d", len(after.Conversations), len(after.NeedsYou), len(before.Conversations), len(before.NeedsYou))
	}
	if again, _ := r.svc.DiscoverHeld(r.project.ID, model.HarnessClaude); len(again.Sessions) != 1 {
		t.Errorf("the look adopted the session: %+v", again)
	}
	for _, c := range []struct{ project, harness, session string }{
		{"no-such-project", model.HarnessClaude, "terminal-1"},
		{r.project.ID, "no-such-harness", "terminal-1"},
		{r.project.ID, model.HarnessClaude, ""},
	} {
		if _, err := r.svc.Look(c.project, c.harness, c.session); err == nil {
			t.Errorf("%+v: no refusal", c)
		}
	}
}
