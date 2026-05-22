# Source connectors

Each subpackage implements one external data source per the four-layer split
tracked in `~/Documents/main/wiki/politics/data-sources/0X *.md` files.

| Package | Source | Current status | Notes |
|---|---|---|---|
| `httpx` | shared HTTP client | shipped | retry, per-host rate limit, RawSink hook |
| `lws` | Washington Legislative Web Services (SOAP/XML) | shipped | live-tested via `-tags=integration` |
| `csi` | Committee Sign In | shipped | read-only; never call /Testimony/Add |
| `committeeschedules` | Committee Schedules ASP.NET app | shipped | search-form date params ignored without CSRF (Phase 0 finding) |
| `tvw` | TVW WordPress + Invintus REST | shipped | use `wp/v2/invintus_video` not `tvw/v1/schedule` for historical sampling |
| `pdc` | data.wa.gov Socrata | shipped | preserves raw fields (PDC schema drifted 2024-10-30) |
| `socrata` | shared Socrata/Tyler API helper | shipped | catalog, metadata, resource query, pagination |
| `datawa` | general data.wa.gov / DES contracts | initial real client | contract dataset constants, catalog/query via shared Socrata, contract normalization |
| `seattle` | Seattle Open Data Socrata | initial real client | priority housing/permit/budget dataset constants, query via shared Socrata, permit normalization |
| `kingcounty` | King County Open Data Socrata | initial real client | priority parcel/election/health/safety dataset constants, query via shared Socrata, parcel normalization |
| `sao` | WA State Auditor ReportSearch | initial real client | gov/audit types, entity lookup, report search, report PDF URLs |
| `seattleauditor` | Seattle City Auditor Missionmark dashboard | initial real client | public S3 dashboard JSON + reports page fetch |
| `census` | Census ACS API | initial real client | ACS table + variable metadata queries |
| `usaspending` | USAspending API v2 | initial real client | award search + agency reference endpoint |
| `fema` | OpenFEMA API v2 | initial real client | OData-style entity queries; WA disaster helper |
| `bls` | BLS public API v2 | initial real client | time-series POST client |
| `hud` | HUD data/catalog/file surfaces | minimal real fetch client | dataset page and known-file fetcher; structured datasets still need per-source parsers |
| `epa` | EPA ECHO/EJScreen surfaces | minimal real fetch client | ECHO facility query helper; detailed endpoints still need per-use-case verification |

## Wiki deltas worth noting

While building Phase 2 we found a few places where the wiki specs disagree
with the live APIs. Captured here so they don't get lost.

### LWS (`data-sources/01`)

- **SOAP namespace** is `http://WSLWebServices.leg.wa.gov/` (capital W-S-L),
  not `http://tempuri.org/` as the spec's pseudocode suggests. The spec
  itself says to verify from `?WSDL` — we did, and this is the verified
  value.
- **`GetSponsors` parameter is `<billId>`** (e.g. `"HB 1234"`), not
  `<billNumber>`. Calling with `<billNumber>` yields a SOAP fault.
- **`GetLegislativeStatusChanges` does not exist** as a single op. The
  WSDL exposes three variants:
  - `GetLegislativeStatusChangesByBillNumber`
  - `GetLegislativeStatusChangesByBillId`
  - `GetLegislativeStatusChangesByDateRange`

### TVW (`data-sources/04`)

- **`tvw/v1/schedule` ignores `start`/`end` params.** It returns a fixed
  ~31-event "currently airing" set regardless of the date window. Use
  `wp/v2/invintus_video?after=…&before=…&orderby=date` for historical
  sampling — that's the production-correct discovery path.

### Committee Schedules (`data-sources/03`)

- The `Home/Search/` form **silently ignores `StartDate`/`EndDate`** without
  an anti-forgery token / session cookie. Operator must capture a real
  browser POST (with `__RequestVerificationToken`) when a date-filtered
  search is needed. Confirmed during Phase 0.

These are retained here as implementation notes for connector maintainers.
