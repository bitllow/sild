package realtime

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/centrifugal/centrifuge"
	"github.com/gin-gonic/gin"
)

// errNoScope means policy admits nothing for this operator, so there is no
// channel set to build and the connection is refused.
var errNoScope = errors.New("no conversation scope")

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

		// Agent (inbox) connection: tenant channels, not conversations. Agents
		// aren't conversation members, and an operator observes the whole queue,
		// so the set comes from policy rather than membership (§5.1).
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
		// User tokens reach no agent channel, so internal notes physically cannot
		// reach a client (§5.6).
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
// connection (§5.2): the agent's user channel, the tenant agents channel, and the
// tenant peer channel for operators whose scope admits peer conversations. The
// agent must be a real admin in the tenant.
//
// The set is fixed per connection — it does not grow with the queue, so a
// conversation created after connect needs no re-subscription.
//
// Visibility is decided by policy.Scope, not re-derived here. REST and the socket
// therefore cannot disagree about what an operator may see — two implementations
// of one policy is how a socket leaks what an endpoint refuses.
func agentSubscriptions(ctx context.Context, st store.Store, tenantID, adminID string) (map[string]centrifuge.SubscribeOptions, error) {
	admin, err := st.Admins().Get(ctx, tenantID, adminID)
	if err != nil {
		return nil, err
	}
	roles, err := st.RoleAssignments().ListByAdmin(ctx, tenantID, adminID)
	if err != nil {
		return nil, err
	}
	scope := policy.Scope(principal.ForAdmin(admin, roles), policy.ConversationsList)
	if scope.DenyAll() {
		return nil, errNoScope
	}
	subs := map[string]centrifuge.SubscribeOptions{
		UserChannel(adminID):    {},
		AgentsChannel(tenantID): {},
	}
	// Operators whose scope admits peer conversations observe the whole peer
	// surface on the single tenant peer channel.
	if scope.AllowsKind(models.KindPeer) {
		subs[PeerChannel(tenantID)] = centrifuge.SubscribeOptions{}
	}
	return subs, nil
}

// Transport paths (§5). The SDKs and the widget hard-code them, so every binary
// that serves the transport mounts these and not its own copy.
const (
	WSPath  = "/v1/ws"
	SSEPath = "/v1/ws/sse"
)

// Mount adds the transport to a gin router, for the binaries that serve it on
// the same listener as REST (sild-dev, sild-standalone).
func (n *Node) Mount(r gin.IRouter) {
	r.GET(WSPath, gin.WrapH(n.WSHandler()))
	r.GET(SSEPath, gin.WrapH(n.SSEHandler()))
}

// Handler returns the HTTP mux serving WS (and SSE for the web widget) (§5).
func (n *Node) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(WSPath, n.WSHandler())
	mux.Handle(SSEPath, n.SSEHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}
