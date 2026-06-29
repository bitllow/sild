package realtime

import (
	"context"
	"net/http"
	"sync"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/centrifugal/centrifuge"
)

// Node is the egress-only Centrifuge node served by sild-ws (§5). It validates
// the user JWT on connect and attaches server-side subscriptions derived from
// membership — the client declares nothing (§5.2).
type Node struct {
	*centrifuge.Node
	cfg     config.Realtime
	runOnce sync.Once
	runErr  error
}

// Run starts the node's broker connection. Idempotent — safe to call from both
// the realtime publisher provider and the serving binary (they share one node).
func (n *Node) Run() error {
	n.runOnce.Do(func() { n.runErr = n.Node.Run() })
	return n.runErr
}

// WSHandler / SSEHandler expose the transport handlers so the all-in-one dev
// binary can mount them on its existing HTTP server.
func (n *Node) WSHandler() http.Handler {
	return centrifuge.NewWebsocketHandler(n.Node, centrifuge.WebsocketConfig{
		CheckOrigin: func(*http.Request) bool { return true },
	})
}

func (n *Node) SSEHandler() http.Handler {
	return centrifuge.NewSSEHandler(n.Node, centrifuge.SSEConfig{})
}

// NewNode builds and configures the node (broker per config, connect handler).
func NewNode(cfg *config.Config, km *auth.KeyManager, st store.Store) (*Node, error) {
	n, err := centrifuge.New(centrifuge.Config{})
	if err != nil {
		return nil, err
	}

	if cfg.Realtime.Broker == "redis" {
		shard, err := centrifuge.NewRedisShard(n, centrifuge.RedisShardConfig{Address: cfg.Realtime.RedisURL})
		if err != nil {
			return nil, err
		}
		broker, err := centrifuge.NewRedisBroker(n, centrifuge.RedisBrokerConfig{Shards: []*centrifuge.RedisShard{shard}})
		if err != nil {
			return nil, err
		}
		pm, err := centrifuge.NewRedisPresenceManager(n, centrifuge.RedisPresenceManagerConfig{Shards: []*centrifuge.RedisShard{shard}})
		if err != nil {
			return nil, err
		}
		n.SetBroker(broker)
		n.SetPresenceManager(pm)
	}

	n.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		claims, err := km.VerifyRealtime(ctx, e.Token)
		if err != nil {
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnauthorized
		}

		// Agent (inbox) connection: subscribe to every conversation that
		// currently carries an assignment — both the conv channel and the
		// agents-only internal channel (§5.1/§5.6) — plus the tenant agents
		// channel for new queue items. Resolved live from the queue, not from
		// membership (agents aren't conversation members).
		if claims.Typ == "agent" {
			subs, err := agentSubscriptions(ctx, st, claims.Tid, claims.Subject)
			if err != nil {
				return centrifuge.ConnectReply{}, centrifuge.ErrorUnauthorized
			}
			return centrifuge.ConnectReply{
				Credentials:   &centrifuge.Credentials{UserID: claims.Subject},
				Subscriptions: subs,
			}, nil
		}

		// Server-side subscriptions: own user channel + every active conversation.
		// User tokens are never subscribed to conv:<id>:internal, so internal
		// notes physically cannot reach a client (§5.6).
		subs := map[string]centrifuge.SubscribeOptions{
			UserChannel(claims.Subject): {},
		}
		members, err := st.Members().ListActiveForUser(ctx, claims.Tid, claims.Subject)
		if err == nil {
			for _, m := range members {
				subs[ConvChannel(m.ConversationID)] = centrifuge.SubscribeOptions{}
			}
		}
		return centrifuge.ConnectReply{
			Credentials:   &centrifuge.Credentials{UserID: claims.Subject},
			Subscriptions: subs,
		}, nil
	})

	n.OnConnect(func(client *centrifuge.Client) {
		// Egress-only: reject any client publish attempt (§1, §5).
		client.OnPublish(func(_ centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			cb(centrifuge.PublishReply{}, centrifuge.ErrorPermissionDenied)
		})
	})

	return &Node{Node: n, cfg: cfg.Realtime}, nil
}

// agentSubscriptions computes the server-side channel set for an inbox agent
// connection (§5.2): the agent's user channel, the tenant agents channel (new
// queue items), and conv:<id> + conv:<id>:internal for every conversation that
// currently carries an assignment. The agent must be a real admin in the tenant.
func agentSubscriptions(ctx context.Context, st store.Store, tenantID, adminID string) (map[string]centrifuge.SubscribeOptions, error) {
	if _, err := st.Admins().Get(ctx, tenantID, adminID); err != nil {
		return nil, err
	}
	subs := map[string]centrifuge.SubscribeOptions{
		UserChannel(adminID):    {},
		AgentsChannel(tenantID): {},
	}
	cids, err := st.Assignments().ConversationIDs(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, cid := range cids {
		subs[ConvChannel(cid)] = centrifuge.SubscribeOptions{}
		subs[ConvInternalChannel(cid)] = centrifuge.SubscribeOptions{}
	}
	return subs, nil
}

// Handler returns the HTTP mux serving WS (and SSE for the web widget) (§5).
func (n *Node) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/ws", n.WSHandler())
	mux.Handle("/v1/ws/sse", n.SSEHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}
