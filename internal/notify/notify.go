// Package notify shows short system notifications. Failures are returned to
// the caller, which decides whether they matter.
package notify

import "github.com/gen2brain/beeep"

// Notifier shows one notification.
type Notifier interface {
	Notify(title, body string) error
}

type nop struct{}

func (nop) Notify(string, string) error { return nil }

// Nop discards notifications. It is a sentinel value, like io.Discard.
var Nop Notifier = nop{}

type system struct{}

func (system) Notify(title, body string) error {
	return beeep.Notify(title, body, "")
}

// System returns a Notifier that shows native desktop notifications.
func System() Notifier { return system{} }
