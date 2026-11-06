// Package recordings holds the short voice clips that ship with the app, so a
// completion can be announced without a system voice being installed. They are
// the four clips the mac app bundles: two Chinese and two English phrases, one
// male and one female voice each.
package recordings

import "embed"

//go:embed wav/*.wav
var assets embed.FS

// Recording is one bundled clip. ID is what preferences store and never
// changes; Language is "zh" or "en", which is what the panel groups by.
type Recording struct {
	ID       string
	Language string
}

// DefaultID is the clip a producer is announced with until the user picks
// another: the mac app's own default.
const DefaultID = "message-reminder-male"

// catalog is in the order the panel shows: Chinese first, then English, a male
// and a female voice in each.
var catalog = []Recording{
	{ID: "message-reminder-male", Language: "zh"},
	{ID: "message-reminder-female", Language: "zh"},
	{ID: "check-your-messages-male", Language: "en"},
	{ID: "check-your-messages-female", Language: "en"},
}

// All returns the bundled clips in panel order.
func All() []Recording { return append([]Recording(nil), catalog...) }

// Lookup finds a clip by ID.
func Lookup(id string) (Recording, bool) {
	for _, r := range catalog {
		if r.ID == id {
			return r, true
		}
	}
	return Recording{}, false
}

// Bytes returns the embedded WAV.
func (r Recording) Bytes() ([]byte, error) {
	return assets.ReadFile("wav/" + r.ID + ".wav")
}
