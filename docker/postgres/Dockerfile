# Postgres for local ENVY trials.
#
# Self-contained: it bakes the dev credentials the ENVY API expects, so it runs
# with no extra configuration. The API (envyd) connects via ENVY_DB_URL and runs
# its own migrations on startup, so there is no schema to seed here.
#
# Build & run (or just `make db` from the repo root):
#   docker build -t envy-postgres docker/postgres
#   docker run --rm -d --name envy-pg -p 5433:5432 envy-postgres
#
# Then point the API at it (note the host port):
#   ENVY_DB_URL=postgres://envy:envy@localhost:5433/envy?sslmode=disable
FROM postgres:16-alpine

# Local-trial defaults — NOT for production. Well-known dev creds that match the
# ENVY_DB_URL documented above and in the Makefile.
ENV POSTGRES_USER=envy \
    POSTGRES_PASSWORD=envy \
    POSTGRES_DB=envy

EXPOSE 5432

# Lets `docker run` / compose wait until the DB accepts connections before the
# API tries to migrate.
HEALTHCHECK --interval=5s --timeout=3s --retries=10 --start-period=10s \
  CMD pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" -h 127.0.0.1 || exit 1
