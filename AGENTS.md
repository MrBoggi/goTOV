# goTOV agent profile

Read `../goTOV-Interfaces/AGENTS.md` and the relevant role playbook in
`../goTOV-Interfaces/agents/roles/` first. This file supplies goTOV-specific
facts; the shared playbooks supply the working method.

## Service boundary

goTOV is the Go backend for brewery control, OPC UA, Brewfather, fermentation,
HTTP, and WebSocket APIs. It is safety-sensitive: unavailable OPC UA must be
visible and reject control writes safely, but must not make the healthy API
process crash merely because the PLC is offline.

Cross-repository APIs, events, PLC/ADS layouts, firmware protocols, and shared
configuration are contracts in `../goTOV-Interfaces/<domain>/<version>/`.
For brewery ADS input work, use
`../goTOV-Interfaces/ads/v1/go/IMPLEMENTATION_TASK.md` and its test vectors.

## Required implementation checks

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
golangci-lint run ./...
go build ./...
docker build --build-arg BUILD_VERSION=dev --tag gotov:local .
```

Add tests for new behaviour. For an integration change, include the applicable
contract/vector test. Health tests must distinguish process/API health from
availability of optional external dependencies.

## Git and delivery

- Branch from current `main`; use a fresh `feature/`, `fix/`, `refactor/`, or
  `chore/` branch.
- Use Conventional Commits and a PR to `main`. Never commit or merge directly
  to `main`.
- The GitHub-hosted `CI` workflow is the PR quality gate. Do not suppress a
  failing test.

## Production deployment

Merge to `main` triggers `.github/workflows/deploy-production.yml` on runner
`deploy-gotov`. It builds `gotov:yyyy.mm.run` and `gotov:latest` locally on
Husserver, deploys only `gotov`, waits for Docker health, and rolls back on
failure.

Do not use Portainer, SSH, or manual Compose to deploy an application change.
Production configuration and secrets stay on the server. Post-deploy evidence
is a successful workflow, a healthy `gotov` container, and a successful
`http://localhost:18085/health` response. Report an offline PLC as degraded
integration, not a successful control connection.
