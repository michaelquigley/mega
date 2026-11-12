// Package model holds the workspace types. it does no I/O and knows no wire or
// harness protocol.
package model

import "time"

// Project is the stable home for work: a display name and an existing checkout.
type Project struct {
	ID       string
	Name     string
	Checkout string
	// Starred puts the project in the starred group at the top of every
	// browser's tree. a registry written before stars reads as unstarred, and
	// an unstarred project writes no field.
	Starred bool `dd:",+omitempty"`
	// EnvApproved is the operator's approval of the checkout's
	// .mega/environment.yaml: mega runs the file's activation only while its
	// content still has this fingerprint. only the fingerprint is kept, never
	// the content, which may hold a literal value; a changed file's approved
	// version is found again in the checkout's git history. a registry
	// written before approvals reads as nothing approved.
	EnvApproved *EnvApproval `dd:",+omitempty"`
	// EnvOverride is the operator's local choice for this project, over the
	// repository's file: ignore it, or run another activation. none means the
	// file decides.
	EnvOverride *EnvOverride `dd:",+omitempty"`

	// Untrusted is the claude trust notice. it is computed when read and never
	// persisted, because the operator can change it from a terminal at any time.
	Untrusted bool `dd:"-"`
	// InheritedPermissionMode is the claude permission mode the operator's
	// native settings name for the checkout, or empty. like Untrusted it is
	// read each time and never persisted.
	InheritedPermissionMode string `dd:"-"`
	// CheckoutMissing says why the checkout cannot be worked in now, or is
	// empty when it can: it does not exist, is not a directory, or is not
	// readable. like Untrusted it is computed when read and never persisted,
	// since the directory can move or come back at any time.
	CheckoutMissing string `dd:"-"`
}

// Clone returns a copy of p that shares nothing mutable with it: the approval
// and the override are copied, so a reader of a copy can never change the
// authority's state.
func (p Project) Clone() Project {
	c := p
	if p.EnvApproved != nil {
		approved := *p.EnvApproved
		c.EnvApproved = &approved
	}
	if p.EnvOverride != nil {
		override := *p.EnvOverride
		c.EnvOverride = &override
	}
	return c
}

// EnvApproval is an approved .mega/environment.yaml: its fingerprint, and
// when the operator approved it.
type EnvApproval struct {
	Fingerprint string
	At          time.Time
}

// EnvOverride is the operator's local override of a project's environment:
// Ignore leaves the repository's file unread and runs no activation; otherwise
// Activate is run in its place. it is the operator's own, so it needs no
// approval.
type EnvOverride struct {
	Ignore   bool   `dd:",+omitempty"`
	Activate string `dd:",+omitempty"`
}
