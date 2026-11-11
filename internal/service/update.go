package service

import "github.com/michaelquigley/mega/internal/model"

// updateBuffer bounds how far a subscriber may fall behind before it is dropped.
const updateBuffer = 256

// Update is one change a subscriber should render. exactly one of Event, Delta,
// Attention or WorkspaceChanged is set. deltas are liveness only and are never
// stored.
type Update struct {
	ConversationID   string       `dd:",+omitempty"`
	Event            *model.Event `dd:",+omitempty"`
	Delta            string       `dd:",+omitempty"`
	Attention        string       `dd:",+omitempty"`
	WorkspaceChanged bool         `dd:",+omitempty"`
}
