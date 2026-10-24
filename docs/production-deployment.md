# Production deployment

goTOV is built and deployed locally on the Bryggeri Docker server when a pull request is merged to `main`.

Each deployment creates `gotov:<yyyy.mm.github-run-number>` and `gotov:latest`. The immutable tag supports manual rollback.

The deployment preserves the existing `go-tov_gotov_data` volume and `go-tov_gotaplist_net` network, and manages only the `gotov` container.

## Server configuration

The production environment variables are stored only on the Docker server at:

```text
/home/gotapdeploy/bryggeri-deploy/gotov/production.env
```

This file contains the OPC UA and Brewfather credentials. It must be owned by `gotapdeploy`, mode `600`, and must never be committed or copied into a GitHub Actions log.

## Runner

The repository needs a GitHub self-hosted runner on the Docker server with the labels:

```text
self-hosted,linux,x64,deploy-gotov
```

## Verify and roll back

```bash
docker inspect --format '{{.State.Health.Status}}' gotov
curl --fail http://127.0.0.1:18085/health
docker image ls gotov
```

To roll back, select a retained tag and run:

```bash
GOTOV_IMAGE=gotov:2026.10.42 docker compose --env-file /home/gotapdeploy/bryggeri-deploy/gotov/production.env --project-name go-tov --file deploy/compose.production.yml up --detach --no-deps gotov
```
