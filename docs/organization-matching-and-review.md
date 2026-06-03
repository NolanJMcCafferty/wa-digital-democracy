# Organization matching and entity review

Last updated: 2026-06-03.

This document explains how WA Digital Democracy turns noisy organization strings
from public testimony and public-record datasets into reviewed organization
links. It is the organization/entity counterpart to
[`hearing-diarization-and-review.md`](hearing-diarization-and-review.md).

The short version:

1. Daily loads IRS BMF and PDC employer reference data before hearing ingest.
2. `ingest-hearings` ingests CSI testifier rows, including raw organization
   strings from sign-ins.
3. The hearing-scoped `PopulateOrganizations` step seeds canonical
   `organization` rows from those CSI strings and links matching `testifier`
   rows back to them.
4. `verify-organizations` can promote seeded CSI organizations to `confirmed`
   when there is a unique IRS BMF or PDC employer normalized-name match.
5. `generate-vendor-entity-matches` runs after `verify-organizations` and
   creates reviewable `vendor_entity_match_candidate` rows by normalized-name
   matching against PDC, CSI testimony, and optional DataWA/FiscalWA/Federal
   source-context rows.
6. Humans use `/admin/review/entities` to confirm, reject, or leave candidates
   in `needs_review`.
7. Public pages show confirmed organizations and confirmed reviewed context;
   unreviewed candidates and merely possible organizations are not public facts.

## Why this exists

Public records describe the same organization in many incompatible ways:

- CSI testifiers type freeform names: `WA Realtors`, `Washington Realtors`,
  `Washington Association of Realtors`, etc.
- PDC lobbying data has employer names.
- DataWA/FiscalWA/USAspending rows use contractor, vendor, customer, or
  recipient names.
- Deepgram may detect organization mentions in hearing speech.

The system therefore separates:

- raw source strings;
- normalized names used only for candidate generation;
- canonical organization rows;
- reviewable match candidates;
- explicit human/system decisions;
- public display through confirmed reviewed matches only.

Normalized-name equality is a starting point, not truth.

## Core tables

### Canonical organizations

- `organization`
  - Canonical organization row used by public pages.
  - Initially seeded from CSI testimony organization strings.
  - Has aliases, match confidence, match notes, and optional verification
    metadata such as `verified_at`, `verification_source`, `irs_bmf_ein`, and
    `pdc_lobbyist_employer_id`.
  - `canonical_name` is unique. Re-running an upsert merges aliases rather than
    replacing them, so later CSI/source passes do not wipe reviewed aliases.

- `testifier.normalized_org_id`
  - Links CSI testifier rows to an `organization` when the source string has
    been seeded/matched.
  - The FK is `ON DELETE SET NULL`, so pruning junk organizations does not
    delete testimony rows.

### Cross-source validation tables

- `pdc_employer`
  - PDC lobbyist-employer records from data.wa.gov dataset `xhn7-64im`.
  - Used as an authoritative public registry for lobbying-employer context and
    for verifying seeded organizations.

- `irs_bmf_organization`
  - IRS BMF Washington nonprofit extract.
  - Used as a cross-source validation source where ingested.

### Reviewable match tables

- `vendor_entity_match_candidate`
  - Candidate link between one public-record source row/name and one canonical
    `organization`.
  - Contains source kind/table/row IDs, `source_name`, `normalized_name`,
    `organization_id`, candidate confidence, evidence strings, and optional
    stable source identifiers.

- `vendor_entity_match_decision`
  - Human/system decision for one candidate.
  - Decision values: `confirmed`, `rejected`, `needs_review`.
  - Stores reviewed confidence, reviewer, notes, and timestamp.

- `reviewed_vendor_entity_match`
  - View exposing only `confirmed` decisions.
  - Public/API consumers should read this view instead of raw candidates.

## Name normalization

Database-side normalization is handled by `wa_dd_normalize_entity_name(raw TEXT)`
from `db/migrations/0010_vendor_entity_resolution.sql`.

It:

- uppercases;
- converts `&` to `AND`;
- splits on non-alphanumeric characters;
- removes common legal suffix / stop tokens such as `THE`, `INC`, `LLC`,
  `CORPORATION`, `ASSOCIATION`;
- rejoins remaining tokens.

Go-side matching also uses helpers in `internal/entitymatch` to:

- dedupe evidence strings;
- estimate candidate confidence;
- avoid known false-positive risks;
- compare canonical organization names and aliases.

Normalization is intentionally conservative. A normalized-name match creates a
candidate; it does not automatically mean two records are the same entity.

## Aliases

Aliases live on `organization.aliases` and are used anywhere the system compares
organization names:

- `populate-organizations` reuses an existing organization when the raw CSI
  organization string exactly matches the canonical name or an alias
  case-insensitively.
- `verify-organizations` checks canonical name plus aliases against IRS BMF and
  PDC employer normalized names.
- `generate-vendor-entity-matches` joins source names against canonical names
  and aliases by normalized name.
- Confirming an entity-match decision appends the candidate `source_name` as an
  alias when it differs from the canonical name and is not already present.

Aliases are not inferred from acronyms automatically. For example, `WHA` does
not become `World Health Association` unless `WHA` has been added as an alias,
usually by confirming a reviewed source-name match or by an explicit
organization upsert.

## Seeding organizations from CSI

The normal hearing pipeline calls `PopulateOrganizations` as its final step.
It can also be run directly:

```sh
go run ./cmd/wa-dd populate-organizations
```

What it does:

1. scans distinct non-empty CSI `testifier.raw_organization` strings;
2. filters junk/self-descriptor strings where possible;
3. upserts canonical `organization` rows;
4. links matching `testifier.normalized_org_id` rows;
5. marks organizations as verified when cross-source validation is available.

Progress output includes processed candidate count, organizations upserted,
linked testifier rows, skipped junk names, and verified count.

### Pruning junk organizations

```sh
go run ./cmd/wa-dd prune-junk-organizations --dry-run
go run ./cmd/wa-dd prune-junk-organizations
```

Use this when earlier seeding created placeholder/self-descriptor organizations.
Because `testifier.normalized_org_id` is `ON DELETE SET NULL`, pruning does not
remove testimony rows.

## Source-context ingests

These commands add external public-record rows that can later be linked to
canonical organizations through reviewable candidates.

In the normal daily chain, IRS BMF and PDC employers run before
`ingest-hearings` so CSI organization seeding can verify against fresh reference
data. The DataWA, FiscalWA, WEBS, and Federal commands are optional bounded
backfills; run `generate-vendor-entity-matches` afterward when you want those
rows to create review candidates.

### PDC lobbying employers

```sh
go run ./cmd/wa-dd ingest-pdc-employers
```

- Source: data.wa.gov / PDC lobbyist-employer dataset `xhn7-64im`.
- Writes `pdc_employer` rows with normalized names.
- Used both for cross-source verification and for reviewed public-record
  context.
- Also writes PDC lobbyist/person affiliation rows. If those rows are ingested
  before the canonical organization exists, they are attached later when the
  organization is seeded or verified.

### IRS BMF Washington nonprofits

```sh
go run ./cmd/wa-dd ingest-irs-bmf-wa
```

- Writes `irs_bmf_organization` rows.
- Useful for validating nonprofit organization names where the dataset has a
  reliable match.

### DataWA / DES contracts and vendors

```sh
go run ./cmd/wa-dd ingest-contracts --fiscal-year 2025 --limit 1000
go run ./cmd/wa-dd ingest-master-contract-sales --limit 1000
go run ./cmd/wa-dd ingest-it-contracts --fiscal-year 2025 --limit 1000
go run ./cmd/wa-dd ingest-webs-vendors --limit 1000
```

These populate bounded source-context tables such as:

- `datawa_contract`
- `datawa_master_contract_sale`
- `datawa_it_contract`
- `datawa_webs_vendor`

Each preserves source dataset/row IDs, raw fields, normalized names, and source
record provenance.

### FiscalWA vendor payments

```sh
go run ./cmd/wa-dd ingest-fiscal-vendor-payments --limit 1000
```

Writes `fiscalwa_vendor_payment` rows with normalized vendor names.

### USAspending Washington awards

```sh
go run ./cmd/wa-dd ingest-usaspending-wa-awards \
  --start-date 2025-10-01 \
  --end-date 2026-09-30 \
  --limit 100
```

Writes `federal_award` rows with normalized recipient names.

## Cross-source verification

After loading PDC employers and/or IRS BMF rows, run:

```sh
go run ./cmd/wa-dd verify-organizations --dry-run
go run ./cmd/wa-dd verify-organizations
```

This scans unverified `organization` rows and tries to match their canonical
name/aliases against `irs_bmf_organization` and `pdc_employer` by normalized
name.

Matching is intentionally conservative:

- IRS BMF is checked first. If there is exactly one IRS match, the organization
  is confirmed with `verification_source = 'irs_bmf'` and `irs_bmf_ein` set.
- PDC employers are checked only when IRS has no unique match. If there is
  exactly one PDC match, the organization is confirmed with
  `verification_source = 'pdc_employer'` and `pdc_lobbyist_employer_id` set.
- If either source has multiple normalized-name matches, that source is treated
  as ambiguous and does not confirm the organization.
- If neither source has a unique match, the organization remains `possible`.

The PDC and IRS tables are reference tables; their ingests do not create
canonical `organization` rows by themselves. They verify already-seeded
organizations. With the appropriate option, `verify-organizations` can also
delete unmatched low-confidence organizations; use that carefully and prefer
`--dry-run` first.

## Candidate generation

### Public-record source candidates

```sh
go run ./cmd/wa-dd generate-vendor-entity-matches --limit 10000
```

Despite the "vendor" naming, this covers more than vendors. It builds
candidates from these source kinds:

- `pdc_lobbying_organization`
- `datawa_contract_contractor`
- `datawa_master_contract_vendor`
- `datawa_master_contract_customer`
- `datawa_it_contract_contractor`
- `datawa_it_contract_dba`
- `datawa_webs_vendor`
- `fiscalwa_vendor_payment`
- `federal_award_recipient`
- `csi_testimony_organization`

For each source row, the generator:

1. selects a source name and normalized name;
2. joins against canonical organization names and aliases by normalized name;
3. skips high-risk false positives;
4. computes candidate confidence/evidence;
5. upserts `vendor_entity_match_candidate`;
6. auto-confirms only unique high-confidence matches (`confirmed` or
   `probable`), marking them as `reviewed_by = system:entitymatch`.

Everything else remains `needs_review` until a human decides.

## Manual human review

Review is required before candidate links become public context unless the
system auto-confirmed a unique high-confidence match.

### Review entry point

Use:

```txt
/admin/review/entities
```

The page supports filtering by:

- decision: `needs_review`, `confirmed`, `rejected`, or all;
- source kind, including PDC, DataWA, FiscalWA, Federal award, and CSI
  testimony organization strings. The API/UI still knows how to display
  Deepgram organization-mention candidate context if such rows exist, but the
  current daily candidate generator does not create them.

Each candidate card shows:

- source kind and source row identifiers;
- source name;
- canonical organization candidate;
- candidate confidence;
- evidence strings;
- existing decision state;
- transcript context for Deepgram organization mentions where available.

### Review actions

- **Confirm**
  - Writes/updates `vendor_entity_match_decision` with `decision = confirmed`.
  - Appends the candidate `source_name` to `organization.aliases` when it is a
    distinct name, so future CSI/source rows can reuse the canonical org.
  - Candidate appears in `reviewed_vendor_entity_match`.
  - Public/API context panels may use it.

- **Reject**
  - Writes/updates `decision = rejected`.
  - Candidate does not appear in `reviewed_vendor_entity_match`.

- **Needs review**
  - Keeps or returns a candidate to `needs_review`.
  - Use when evidence is insufficient or a second reviewer should decide.

### What reviewers should check

1. **Name identity**
   - Are the source name and canonical organization actually the same entity?
   - Beware shared acronyms, generic names, subsidiaries, locals/chapters, and
     similarly named associations.

2. **Source kind semantics**
   - A PDC lobbying employer match means the organization appears in lobbying
     employer records.
   - A contract/vendor/payment/award match means there is public-record context
     for a similarly named entity.
  - A Deepgram mention, when present, means someone said something that a model
    detected as an organization name.

3. **Evidence quality**
   - Exact normalized-name match is good but not always enough.
   - Prefer source IDs, official URLs, known aliases, and surrounding transcript
     context.
   - If uncertain, leave as `needs_review` or reject.

4. **Public wording risk**
   - Do not imply causation. A lobbying/contract/payment/award record is related
     public-record context, not proof that money caused a bill position.
   - Do not publish unreviewed Deepgram organization mentions as confirmed org
     appearances.

## Public display rules

Public organization and bill/hearing context should use reviewed links only:

- public `/organizations` list → confirmed `organization` rows only;
- confirmed decision → may appear as public-record context;
- rejected decision → not public context;
- needs_review / no decision → internal candidate only;
- Deepgram entity mention → evidence only until confirmed;
- source rows retain source URLs/records so readers can audit.

The main public/API safety boundaries are confirmed `organization` rows for the
organization directory and `reviewed_vendor_entity_match` for source-context
links. Raw `vendor_entity_match_candidate` rows are internal review artifacts.

## Quality-control checklist

Before relying on organization/entity context publicly:

- [ ] CSI testimony organizations have been populated with
      `populate-organizations` / pipeline `PopulateOrganizations`.
- [ ] Junk placeholder organizations have been pruned or left unlinked.
- [ ] Relevant source-context rows have been ingested.
- [ ] `verify-organizations` has been run after CSI organization seeding.
- [ ] `generate-vendor-entity-matches` has been run after source-context ingest
      and `verify-organizations`.
- [ ] High-confidence candidates have explicit decisions.
- [ ] Confirmed source-name variants have landed in `organization.aliases`.
- [ ] Public organization directory reads confirmed `organization` rows, and
      public source-context panels read confirmed matches through
      `reviewed_vendor_entity_match`.
- [ ] UI language describes public-record context without implying causation.

## Common failure modes

### Too many junk organizations

Run `prune-junk-organizations --dry-run`, inspect examples, then run the real
command if the candidates are clearly placeholders/self-descriptors.

### No candidates generated

Check that source-context tables have rows and normalized names:

```sql
SELECT COUNT(*) FROM pdc_employer WHERE normalized_name IS NOT NULL;
SELECT COUNT(*) FROM datawa_contract WHERE normalized_contractor_name IS NOT NULL;
SELECT COUNT(*) FROM organization WHERE wa_dd_normalize_entity_name(canonical_name) IS NOT NULL;
```

Also check whether names differ in ways normalization cannot bridge. Confirm a
review candidate or add aliases to `organization.aliases` rather than loosening
matching globally.

### Obvious candidate stuck as needs_review

Confirm it in `/admin/review/entities` or with:

```sh
go run ./cmd/wa-dd decide-entity-match \
  --candidate-id <id> \
  --decision confirmed \
  --confidence confirmed \
  --reviewed-by nolan \
  --notes "Exact source name match."
```

Confirming the decision also stores the candidate source name as an alias on the
organization when it is distinct from the canonical name.

### Deepgram mention is misleading

Reject the candidate. Provider entity detection is evidence only and can mistake
phrases, agencies, or topics for organization names. Deepgram organization
mention candidates are not generated by the current daily pipeline, but this
rule still applies to legacy or manually inserted candidates.

### Public page missing expected context

Confirm the candidate appears in the reviewed view:

```sql
SELECT *
  FROM reviewed_vendor_entity_match
 WHERE organization_id = <organization_id>
 ORDER BY reviewed_at DESC;
```

If it only appears in `vendor_entity_match_candidate`, it has not been confirmed
and should not be public yet.

## Useful SQL snippets

Candidates needing review:

```sql
SELECT c.id, c.source_kind, c.source_name, o.canonical_name,
       c.candidate_confidence, COALESCE(d.decision::text, 'needs_review') AS decision
  FROM vendor_entity_match_candidate c
  JOIN organization o ON o.id = c.organization_id
  LEFT JOIN vendor_entity_match_decision d ON d.candidate_id = c.id
 WHERE COALESCE(d.decision::text, 'needs_review') = 'needs_review'
 ORDER BY c.updated_at DESC
 LIMIT 100;
```

Confirmed context for an organization:

```sql
SELECT source_kind, source_name, match_confidence, review_notes, reviewed_at
  FROM reviewed_vendor_entity_match
 WHERE organization_id = <organization_id>
 ORDER BY reviewed_at DESC;
```

CSI testifier sign-ins for an organization:

```sql
SELECT t.raw_organization, COUNT(*) AS mention_count
  FROM testifier t
 WHERE t.normalized_org_id = <organization_id>
 GROUP BY t.raw_organization
 ORDER BY mention_count DESC, t.raw_organization;
```

Deepgram organization candidates with transcript context, if legacy/manual
candidates exist:

```sql
SELECT c.id, c.source_name, c.normalized_name, o.canonical_name,
       em.tvw_event_id, em.start_ms, em.end_ms, em.confidence
  FROM vendor_entity_match_candidate c
  JOIN organization o ON o.id = c.organization_id
  JOIN entity_mention em ON em.id = c.source_pk
 WHERE c.source_kind = 'deepgram_organization_mention'
 ORDER BY em.confidence DESC NULLS LAST
 LIMIT 100;
```

## Safety principles

- Raw organization strings are not canonical identities by themselves.
- Normalized-name matches create candidates, not facts.
- Aliases are reviewed identity hints, not automatic acronym expansion.
- Confirmed `reviewed_vendor_entity_match` rows are the public boundary.
- Deepgram organization mentions are evidence only until reviewed.
- Prefer false negatives over false positive organization claims.
- Public language should say “public-record context,” not causation.
- Keep source IDs, source URLs, raw fields, and review notes so matches can be
  audited later.
