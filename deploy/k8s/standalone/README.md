# Standalone k8s variant

One Deployment running `sild-standalone` — REST, realtime and the background jobs
in one process on one port — instead of the four in `deploy/k8s/`. Same image,
same ConfigMap and Secret; the topology is chosen by `command`, not by build.

The split manifests one directory up are not deprecated. They are the other end
of the same scale: use them when api, ws and worker need to scale on their own
axes. Start here, move there when a dimension actually hurts.

```sh
kubectl apply -f deploy/k8s/00-namespace.yaml
# The pod mounts this to reach the bucket; without it, it never starts.
kubectl -n sild create secret generic sild-gcs-key \
  --from-file=key.json=/path/to/service-account-key.json
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

```sh
kubectl -n sild scale deploy/sild-standalone --replicas=3
```

Nothing else changes: attachments and archived conversations go to the bucket in
`05-config.yaml`, so every replica reads what any other wrote. Add an HPA if you
want one — long-lived WS connections make scale-down disruptive, so keep the
stabilization window generous (clients reconnect and catch up, §5.4):

```yaml
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

Set `STORAGE_BUCKET`, then give the pods' service account the two IAM grants and
the CORS policy in [The bucket](../../../docs/deployment.md#the-bucket) — signed
URLs go through IAM SignBlob, so workload identity is enough and no key file is
needed. Skipping the CORS policy leaves signed URLs that fail in the browser only.

The jobs need no attention: the outbox claim and the archive lease make every
replica safe to run them (ARCHITECTURE §4). If you would rather keep serving pods
free of background work, set `SILD_JOBS: ""` here and run a second Deployment
with `SILD_JOBS: webhook,archive` — or just use `deploy/k8s/32-worker.yaml`.
