package service

// Subscription delivers updates to one consumer. its channel closes when the
// consumer falls too far behind or the service closes; the consumer recovers by
// reconnecting and re-reading confirmed state, so nothing waits on a slow reader.
type Subscription struct {
	svc     *Service
	updates chan Update
}

// Updates returns the delivery channel.
func (sub *Subscription) Updates() <-chan Update {
	return sub.updates
}

// Close ends the subscription. it is safe to call more than once.
func (sub *Subscription) Close() {
	sub.svc.mu.Lock()
	defer sub.svc.mu.Unlock()
	sub.svc.drop(sub)
}
