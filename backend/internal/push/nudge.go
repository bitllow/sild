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

// genericTitle is what a nudge says when the tenant has turned the sender name
// off — the notification still has to say something.
const genericTitle = "New message"

// previewLimit keeps a body short enough that a lock screen shows the start of
// it rather than a truncation of the middle.
const previewLimit = 180

// Compose turns a message into notification text under the tenant's settings.
//
//	sender  body   title      text
//	off     off    New message
//	off     on     New message  <preview>
//	on      off    <sender>     New message
//	on      on     <sender>     <preview>
func Compose(s Settings, senderName, body string) (title, text string) {
	title, text = genericTitle, ""
	if s.IncludeSender && senderName != "" {
		title = senderName
		text = genericTitle
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
