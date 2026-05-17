# Phase 0 feasibility spike

Throwaway Go programs that answer the four MVP-blocking unknowns from the wiki
(`Washington Digital Democracy - First Page Implementation Blueprint.md`,
§"Known blockers / open questions") before we build the real pipeline.

## Programs

| Question | Program | Output |
|---|---|---|
| What share of TVW legislative-committee events have `captionPath` VTT? | `tvw-captions` | `data/spike/tvw-captions.json` |
| What share of Committee Schedules meetings expose a TVW event ID directly? | `sched-tvw-mapping` | `data/spike/sched-tvw-mapping.json` |
| How far back does CSI retain testifier sign-in data? | `csi-history` | `data/spike/csi-history.json` |
| Is written testimony accessible via a public no-auth endpoint? | (manual) | `data/spike/written-testimony.md` |

## Running

```bash
# TVW captions — needs the public Invintus embedder key (see below)
INVINTUS_EMBEDDER_KEY=<key> \
  go run ./cmd/wa-dd-spike/tvw-captions \
    -days 30 \
    -out data/spike/tvw-captions.json

# Committee Schedules → TVW event mapping
go run ./cmd/wa-dd-spike/sched-tvw-mapping \
  -days 30 \
  -out data/spike/sched-tvw-mapping.json

# CSI history depth
go run ./cmd/wa-dd-spike/csi-history \
  -out data/spike/csi-history.json
```

After running all three, hand-roll `data/spike/REPORT.md` summarizing findings.

## Finding the Invintus embedder key

The Invintus player ships a public `wsc-api-key` in browser JS. To extract:

1. Visit any TVW video page, e.g. `https://tvw.org/watch?eventID=2026041162`.
2. Open DevTools → Network. Reload.
3. Look for `POST https://api.v3.invintus.com/v2/Event/getDetailed`.
4. Copy the `wsc-api-key` request header value.
5. Treat it as **public** (it ships to all browsers) but rate-limit aggressively.

Source: `data-sources/04 TVW Invintus Client.md` lines 49, 105–110.

## Exit criterion

If the TVW caption-coverage spike reports <40% coverage for legislative
committees, **pause** before building the rest of the pipeline — the entire
"What was said?" surface depends on this signal.
