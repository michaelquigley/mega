package service

import (
	"cmp"
	"slices"

	"github.com/michaelquigley/mega/internal/model"
)

// Read moves the conversation's reading position to seq, never back and never
// past its last event. it acknowledges what was read and nothing more: a
// pending request stays pending until it is answered.
func (s *Service) Read(conversationID string, seq int64) error {
	if seq < 0 {
		return newError(CodeInvalid, "the reading position cannot be negative")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, err := s.conversationLocked(conversationID)
	if err != nil {
		return err
	}
	l, err := s.liveLocked(conversationID)
	if err != nil {
		return err
	}
	seq = min(seq, l.log.LastSeq())
	if seq <= conv.ReadSeq {
		return nil
	}
	return s.updateConversation(conversationID, func(c *model.Conversation) {
		if seq > c.ReadSeq {
			c.ReadSeq = seq
		}
	})
}

// needsYou orders the conversations waiting on the operator: every one with a
// pending request, then every one with unread output, each oldest first by the
// time of the fact that put it there.
func needsYou(conversations []ConversationView) []string {
	var waiting []ConversationView
	for _, conv := range conversations {
		if conv.Attention == model.AttentionRequest || conv.Attention == model.AttentionUnread {
			waiting = append(waiting, conv)
		}
	}
	slices.SortStableFunc(waiting, func(a, b ConversationView) int {
		if a.Attention != b.Attention {
			if a.Attention == model.AttentionRequest {
				return -1
			}
			return 1
		}
		if c := a.AttentionAt.Compare(b.AttentionAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	ids := make([]string, 0, len(waiting))
	for _, conv := range waiting {
		ids = append(ids, conv.ID)
	}
	return ids
}
