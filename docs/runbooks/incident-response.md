# Incident Response Runbook

## Overview

This runbook covers common incident scenarios for DevPlatform.

## Severity Levels

| Level | Definition | Response Time | Escalation |
|-------|------------|---------------|------------|
| SEV1 | Complete outage / data loss | 15 min | Page on-call + engineering lead |
| SEV2 | Partial outage / degraded | 30 min | Page on-call |
| SEV3 | Minor issue / no user impact | 4 hours | Slack notification |

## Common Incidents

### High CPU Usage

**Symptoms:** Pod CPU > 80% for 5 min, latency increases

**Steps:**
1. Check Grafana dashboard → `devplatform` → Resource Usage
2. Identify hot path via Tempo traces
3. Check recent deployments: `kubectl rollout history deployment/control-plane -n production`
4. If recent deploy caused it: `kubectl rollout undo deployment/control-plane -n production`
5. If not: scale horizontally: `kubectl scale deployment/control-plane --replicas=10 -n production`

**Escalation:** SEV2 if sustained > 15 min

### Database Connection Pool Exhaustion

**Symptoms:** Logs show "connection pool exhausted", 5xx errors

**Steps:**
1. Check connection count: `kubectl exec -it <pod> -- psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity"`
2. Check for long-running queries
3. Restart affected pods: `kubectl rollout restart deployment/control-plane -n production`
4. Increase pool size in ConfigMap if needed

**Escalation:** SEV1 if no recovery in 10 min

### Message Queue Backup

**Symptoms:** NATS/Redis queue depth growing, processing lag

**Steps:**
1. Check consumer lag in Grafana
2. Scale consumers: `kubectl scale deployment/worker --replicas=5`
3. Check for poison messages (dead letter queue)
4. Verify downstream dependencies healthy

**Escalation:** SEV2 if backlog > 1M messages

### Memory Leak

**Symptoms:** OOMKills, memory climbing over time

**Steps:**
1. Check Grafana memory panel
2. Capture heap profile if Go: `kubectl exec <pod> -- curl localhost:6060/debug/pprof/heap > heap.prof`
3. Analyze with pprof
4. Deploy fix or restart pods as mitigation

**Escalation:** SEV2

### AI Job Queue Stuck

**Symptoms:** AI jobs stuck in "pending" or "running" for extended period

**Steps:**
1. Check worker logs: `kubectl logs -l app=worker -n production`
2. Check NATS queue depth
3. Check AI provider API status (OpenAI/Anthropic/Ollama)
4. Restart worker pods: `kubectl rollout restart deployment/worker -n production`
5. Clear stuck jobs via admin API if needed

**Escalation:** SEV2 if > 50 jobs stuck

### Frontend Build/Deploy Failure

**Symptoms:** Frontend returns 5xx, stale assets, build errors in CI

**Steps:**
1. Check CI pipeline logs for build errors
2. Verify `npm run build` passes locally
3. Check CDN/cache invalidation
4. Rollback to previous frontend deployment: `kubectl rollout undo deployment/frontend -n production`

**Escalation:** SEV2

## Recovery Verification

After mitigation:
- [ ] Error rate back to baseline (< 0.1%)
- [ ] Latency back to SLO (p99 < 500ms)
- [ ] No ongoing alerts
- [ ] User-reported issues resolved

## Post-Incident

1. Create incident timeline document
2. Schedule post-mortem (within 48h for SEV1/SEV2)
3. Create follow-up issues for:
   - Root cause elimination
   - Detection improvement
   - Prevention measures

## Contacts

- **On-call**: PagerDuty → `#oncall-devplatform`
- **Engineering Lead**: @lead in Slack
- **Incident Commander**: @ic in Slack