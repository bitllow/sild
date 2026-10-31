package gormstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

func TestListActiveConversationsForActor(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			const tenant = "t_actor_convs"
			st := newQueueStore(t, dbc, tenant)
			ctx := context.Background()
			actor, other := "adm_actor", "adm_other"
			now := time.Now()
			left := now
			seed := []struct {
				conv  string
				queue string
				kind  models.ConversationKind
				who   string
				left  *time.Time
			}{
				{"c_actor_triage", "triage", models.KindSupport, actor, nil},
				{"c_actor_peer", models.QueueSupport, models.KindPeer, actor, nil},
				{"c_actor_left", "triage", models.KindSupport, actor, &left},
				{"c_other", "triage", models.KindSupport, other, nil},
			}
			for _, s := range seed {
				if err := st.Conversations().Create(ctx, &models.Conversation{
					ID: s.conv, TenantID: tenant, Queue: s.queue, Kind: s.kind, Status: models.ConversationOpen, CreatedAt: now,
				}); err != nil {
					t.Fatal(err)
				}
				who := s.who
				if err := st.Members().Add(ctx, &models.ConversationMember{
					ID: "m_" + s.conv, TenantID: tenant, ConversationID: s.conv, MemberKind: models.MemberAgent,
					InternalActorID: &who, ConvRole: models.RoleClient, JoinedAt: now, LeftAt: s.left,
				}); err != nil {
					t.Fatal(err)
				}
			}

			got, err := st.Members().ListActiveConversationsForActor(ctx, tenant, actor)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]store.ActorConversation{
				"c_actor_triage": {ConversationID: "c_actor_triage", Queue: "triage", Kind: models.KindSupport},
				"c_actor_peer":   {ConversationID: "c_actor_peer", Queue: models.QueueSupport, Kind: models.KindPeer},
			}
			if len(got) != len(want) {
				t.Fatalf("got %+v, want %+v", got, want)
			}
			for _, c := range got {
				if want[c.ConversationID] != c {
					t.Fatalf("got %+v, want %+v", c, want[c.ConversationID])
				}
			}
		})
	}
}
