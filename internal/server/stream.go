package server

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"
)

// heartbeatFrame lets the browser see that a quiet stream is still alive, which
// control-frame pings cannot do from page script.
type heartbeatFrame struct {
	Heartbeat time.Time
}

// stream relays service updates to one browser. the subscription starts before
// the upgrade completes, so a client that reads confirmed state once connected
// cannot miss a change made in between.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	sub := s.svc.Subscribe()
	defer sub.Close()

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		dl.Debugf("upgrading stream: %v", err)
		return
	}
	defer conn.Close()

	// reading is what processes pong and close frames.
	readWait := s.heartbeat * heartbeatsPerRead
	conn.SetReadLimit(streamReadLimit)
	conn.SetReadDeadline(time.Now().Add(readWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readWait))
	})
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-gone:
			return
		case update, ok := <-sub.Updates():
			if !ok {
				// slow reader or service shutdown; the browser reconnects and re-reads.
				deadline := time.Now().Add(streamWriteWait)
				closing := websocket.FormatCloseMessage(websocket.CloseGoingAway, "")
				if err := conn.WriteControl(websocket.CloseMessage, closing, deadline); err != nil {
					dl.Debugf("closing stream: %v", err)
				}
				return
			}
			if !writeFrame(conn, &update) {
				return
			}
		case now := <-ticker.C:
			deadline := time.Now().Add(streamWriteWait)
			if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
				return
			}
			if !writeFrame(conn, &heartbeatFrame{Heartbeat: now.UTC()}) {
				return
			}
		}
	}
}

func writeFrame(conn *websocket.Conn, frame any) bool {
	data, err := dd.UnbindJSONL(frame)
	if err != nil {
		dl.Errorf("encoding stream frame: %v", err)
		return false
	}
	conn.SetWriteDeadline(time.Now().Add(streamWriteWait))
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		dl.Debugf("writing stream frame: %v", err)
		return false
	}
	return true
}
