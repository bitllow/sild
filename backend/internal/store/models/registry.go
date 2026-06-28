package models

// All returns every model, in dependency order, for AutoMigrate.
func All() []any {
	return []any{
		&Tenant{},
		&TenantSearchableKey{},
		&TenantEmailConfig{},
		&TenantEmailDomain{},
		&APIKey{},
		&AdminUser{},
		&AdminSession{},
		&SigningKey{},
		&Conversation{},
		&ConversationMember{},
		&Assignment{},
		&Message{},
		&MessageAttachment{},
		&EmailThread{},
		&Upload{},
		&ReadReceipt{},
		&PushToken{},
		&WebhookEndpoint{},
		&WebhookEvent{},
		&Outbox{},
		&WebhookDelivery{},
		&ConversationArchive{},
	}
}
