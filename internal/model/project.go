// Package model holds the workspace types. it does no I/O and knows no wire or
// harness protocol.
package model

// Project is the stable home for work: a display name and an existing checkout.
type Project struct {
	ID       string
	Name     string
	Checkout string
	// Starred puts the project in the starred group at the top of every
	// browser's tree. a registry written before stars reads as unstarred, and
	// an unstarred project writes no field.
	Starred bool `dd:",+omitempty"`

	// Untrusted is the claude trust notice. it is computed when read and never
	// persisted, because the operator can change it from a terminal at any time.
	Untrusted bool `dd:"-"`
	// InheritedPermissionMode is the claude permission mode the operator's
	// native settings name for the checkout, or empty. like Untrusted it is
	// read each time and never persisted.
	InheritedPermissionMode string `dd:"-"`
}
