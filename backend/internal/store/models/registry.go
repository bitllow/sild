package models

// All returns every model, in dependency order, for AutoMigrate.
func All() []any {
	return []any{
		&Tenant{},
		&TenantSearchableKey{},
		&TenantEmailConfig{},
		&TenantEmailDomain{},
		&TenantPushConfig{},
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
		&PushOptOut{},
		&WebhookEndpoint{},
		&WebhookEvent{},
		&Outbox{},
		&PushOutbox{},
		&WebhookDelivery{},
		&ConversationArchive{},
		&Brand{},
		&JobLease{},
		&EmailIngest{},
	}
}
