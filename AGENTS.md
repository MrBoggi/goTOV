# goTOV agent profile

Read `../goTOV-Interfaces/AGENTS.md` and the relevant role playbook in
`../goTOV-Interfaces/agents/roles/` first. This file supplies goTOV-specific
facts; the shared playbooks supply the working method.

For cross-repository work, start from the active row in
`../goTOV-Interfaces/agents/work/PRIORITIES.md` and resume from `STATUS.md`.
Load only the cited contract, role, and affected code path; do not rescan the
repository or historical branches when the hand-off evidence is current.

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

Run the relevant checks before opening a PR; run the complete set for backend,
configuration, Docker, or integration changes:

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
- The GitHub-hosted `CI` workflow is the PR quality gate. A passing test command
  is required; do not suppress a failure.

## Production deployment

Merge to `main` triggers `.github/workflows/deploy-production.yml` on the
repository runner labelled `deploy-gotov`. It builds `gotov:yyyy.mm.run` and
`gotov:latest` locally on Husserver, deploys only the `gotov` Compose service,
waits for Docker health, and rolls back to the previous image if needed.

Do not use Portainer, SSH, or a manual Compose command to deploy an application
change. Production configuration and secrets stay on the server. Post-deploy
evidence is: successful workflow, `gotov` is running and healthy, and
`http://localhost:18085/health` returns success. Report an offline PLC as a
degraded integration, not a successful control connection.
