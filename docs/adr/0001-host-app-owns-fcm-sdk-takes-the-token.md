# The host app owns FCM; the SDK takes the token

Tenants own the push project their app is built against, so their app already
integrates Firebase. The Sild mobile SDKs therefore depend on no Firebase
library: the host forwards its push token and any Sild-owned message into the
SDK, and the SDK offers a predicate to tell its messages apart from the host's
own. This is the same shape as the widget's `tokenProvider` — the host supplies
identity and credentials, Sild consumes them — and the same shape Intercom's
SDKs use.

## Considered options

Bundling `firebase-messaging` in the SDK was rejected on a hard constraint, not
a preference. Android resolves exactly one `FirebaseMessagingService` per app,
so a host that already has one and an SDK that ships another produces a silent
manifest-merge conflict where one of them simply never fires. iOS has the same
problem over `didRegisterForRemoteNotificationsWithDeviceToken`. There is also
only ever one push token per app instance, so an SDK-owned token would be the
same string the host already holds.

## Consequences

The integration has obligations the SDK cannot discharge for the host: forward
the token on issue and rotation, forward foreground messages, and create the
notification channel the server payload names.
