# Nudges are sent without a presence check

Fan-out sends to every member's devices and lets the foreground app discard
what it doesn't want, rather than asking the broker who is connected first. The
point of a presence check is to avoid notifying someone who is looking at the
conversation, and both platforms already answer that question locally and more
accurately: a message with a notification payload is handed to the app rather
than displayed when the app is in the foreground — `onMessageReceived` on
Android, `willPresent` on iOS. Presence would tell us about a socket; the
device knows what is on screen.

## Considered options

Gating on Centrifuge presence is what §5.5 of the spec and the `push` package's
own comments describe. It was rejected because it buys nothing the foreground
handoff doesn't already give, while adding a broker read per member per message
and a race the client would have to cover anyway.

Suppressing while backgrounded — the only thing presence could add — would
require data-only messages. On iOS those are silent pushes, which are throttled
and carry no delivery guarantee, so it would trade reliable notifications for a
capability nobody wants.

## Consequences

`PresenceChecker` and `AlwaysOffline` have no remaining purpose and go with
this change; the presence branch in `push.FanOut.Deliver` goes with them.

Nudge text is composed server-side, because a backgrounded device displays what
arrived without running app code. The tenant's sender and body settings are
therefore applied during fan-out, not in the SDK.
