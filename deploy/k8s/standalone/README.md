# Standalone k8s variant

One Deployment running `sild-standalone` — REST, realtime and the background jobs
in one process on one port — instead of the four in `deploy/k8s/`. Same image,
same ConfigMap and Secret; the topology is chosen by `command`, not by build.

The split manifests one directory up are not deprecated. They are the other end
of the same scale: use them when api, ws and worker need to scale on their own
axes. Start here, move there when a dimension actually hurts.

```sh
kubectl apply -f deploy/k8s/00-namespace.yaml
kubectl apply -f deploy/k8s/05-config.yaml
kubectl apply -f deploy/k8s/10-postgres.yaml -f deploy/k8s/11-redis.yaml
kubectl apply -f deploy/k8s/20-migrate.yaml            # schema first, always
kubectl apply -f deploy/k8s/standalone/
```

Then create the first tenant — the standalone pods have no seed data:

```sh
kubectl -n sild exec -it deploy/sild-standalone -- \
  /usr/local/bin/sild-admin tenant create --name "Acme" --admin-email you@acme.com
```

## Scaling

**Switch attachment storage before you add a replica.** The Deployment mounts a
node-local `hostPath`, and `sild-config` asserts `STORAGE_LOCAL_SHARED=true` —
which is a true statement at one replica on one node, and a false one the moment
a second pod is scheduled elsewhere. `config.Validate` takes that assertion at
its word, so it will *not* catch this for you: pods would start happily and
answer 404 for each other's uploads. Set `STORAGE_BACKEND=gcs` (or `s3`) and drop
the volume first.

Then scale, and add an HPA if you want it:

```sh
kubectl -n sild scale deploy/sild-standalone --replicas=3
```

```yaml
# Long-lived WS connections make scale-down disruptive — clients reconnect and
# catch up (§5.4), but a generous stabilization window avoids churn.
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: { name: sild-standalone, namespace: sild }
spec:
  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: sild-standalone }
  minReplicas: 1
  maxReplicas: 6
  metrics:
    - type: Resource
      resource: { name: cpu, target: { type: Utilization, averageUtilization: 70 } }
  behavior:
    scaleDown: { stabilizationWindowSeconds: 600 }
```

It is a snippet rather than a file in this directory on purpose: `kubectl apply -f
deploy/k8s/standalone/` would otherwise install autoscaling over node-local
storage in one command.

The jobs need no attention: the outbox claim and the archive lease make every
replica safe to run them (ARCHITECTURE §4). If you would rather keep serving pods
free of background work, set `SILD_JOBS: ""` here and run a second Deployment
with `SILD_JOBS: webhook,archive` — or just use `deploy/k8s/32-worker.yaml`.
