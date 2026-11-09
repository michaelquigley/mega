// Package model holds the workspace types. it does no I/O and knows no wire or
// harness protocol.
package model

// Project is the stable home for work: a display name and an existing checkout.
type Project struct {
	ID       string
	Name     string
	Checkout string

	// Untrusted is the claude trust notice. it is computed when read and never
	// persisted, because the operator can change it from a terminal at any time.
	Untrusted bool `dd:"-"`
}
