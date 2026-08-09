package push

import "github.com/bitllow/sild/backend/internal/store/models"

// Settings are the tenant's choices about what a nudge may reveal. Applied
// server-side because the composing happens here, not on the device.
type Settings struct {
	IncludeSender bool
	IncludeBody   bool
	// SenderSource picks whose name a support nudge carries.
	SenderSource models.PushSenderSource
}

// GenericKey is what a nudge says when the tenant has turned the sender name off
// — the notification still has to say something. Resolved per recipient, because
// a backgrounded device displays what arrived and runs no app code.
const GenericKey = "push.newMessage"

// previewLimit keeps a body short enough that a lock screen shows the start of
// it rather than a truncation of the middle.
const previewLimit = 180

// Compose turns a message into notification text under the tenant's settings.
// generic is the recipient's own wording of "New message".
//
//	sender  body   title      text
//	off     off    generic
//	off     on     generic    <preview>
//	on      off    <sender>   generic
//	on      on     <sender>   <preview>
func Compose(s Settings, generic, senderName, body string) (title, text string) {
	title, text = generic, ""
	if s.IncludeSender && senderName != "" {
		title = senderName
		text = generic
	}
	if s.IncludeBody {
		if p := preview(body); p != "" {
			text = p
		}
	}
	return title, text
}

// preview trims a body to a single notification line.
func preview(body string) string {
	r := []rune(body)
	if len(r) > previewLimit {
		return string(r[:previewLimit])
	}
	return string(r)
}
