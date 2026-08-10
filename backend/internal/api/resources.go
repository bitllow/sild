package api

// Resource names bind a cursor to the collection that minted it, so a cursor
// from one list is rejected by another rather than silently mispositioning it.
const (
	resourceConversations = "conversations"
	resourceMessages      = "messages"
	resourceContacts      = "contacts"
	resourceTeam          = "team"
	resourceAPIKeys       = "api_keys"
	resourceWebhooks      = "webhooks"
	resourceDeliveries    = "webhook_deliveries"

	resourceTranslationProjects = "translation_projects"
	resourceTranslationKeys     = "translation_keys"
	resourceTranslationReleases = "translation_releases"
)
