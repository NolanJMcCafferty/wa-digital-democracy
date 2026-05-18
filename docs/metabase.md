# Metabase local analytics

The local Docker Compose stack includes optional Metabase for internal analytics
and database exploration.

Start it:

```sh
make analytics
```

Open:

```txt
http://localhost:3001
```

or run:

```sh
make metabase-open
```

## First-run setup

Metabase will ask you to create a local admin account. This is local-only dev
state, not a production account.

When adding the WA Digital Democracy database as a data source, use:

- Database type: PostgreSQL
- Host: `postgres`
- Port: `5432`
- Database name: `wa_dd`
- Username: `wadd`
- Password: `wadd`
- Schema: `public`

Metabase itself stores its application metadata in the same local Postgres
container under schema `metabase`. This is convenient for local development, but
should not be copied to production. In production, Metabase should use a
separate application database and connect to WA Digital Democracy with a
read-only analytics user.

## Useful starter questions

- Bills by prefix and current status
- Hearings by committee over time
- Testimony position counts by bill
- Organizations by number of testimony appearances
- Source records by system and latest fetch time
- Ingestion runs by status and duration
