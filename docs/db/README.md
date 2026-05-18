# Database schema docs

Generate browsable SchemaSpy documentation for the local Postgres schema:

```sh
make db-docs
```

Output is written to `docs/db/schemaspy/` and is intentionally gitignored.
Open `docs/db/schemaspy/index.html` in a browser to inspect tables, columns,
indexes, relationships, and ER diagrams.

Requirements:

- Docker
- local Postgres running via `make up`
- migrations applied via `make migrate-up`

The SchemaSpy container runs on the repo's Docker Compose network and connects
to the `postgres` service using the local development credentials.
