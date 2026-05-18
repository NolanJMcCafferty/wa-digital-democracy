import Link from "next/link";

export default function MethodologyPage() {
  return (
    <article className="prose prose-stone max-w-3xl space-y-8">
      <header className="space-y-3">
        <p className="text-sm font-medium uppercase tracking-wider text-stone-500">
          Methodology
        </p>
        <h1 className="text-4xl font-bold tracking-tight text-stone-900">
          How this works
        </h1>
        <p className="text-lg text-stone-700">
          WA Digital Democracy is source-linked by design. Every public fact
          on the site traces back to an official feed, public record,
          document, video, transcript, or dataset.
        </p>
      </header>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          Architectural ground rules
        </h2>
        <ol className="list-decimal space-y-3 pl-6 text-stone-700">
          <li>
            <strong>Postgres owns truth.</strong> Search, AI summaries, and
            derived analytics are rebuildable from the underlying
            normalized tables.
          </li>
          <li>
            <strong>Raw source records are immutable.</strong> Every API
            response we fetch is stored verbatim under{" "}
            <code>data/raw/&lt;system&gt;/</code> with a content hash.
            Re-fetching identical bytes deduplicates.
          </li>
          <li>
            <strong>Every public fact needs provenance.</strong> Each row
            in a normalized table points at an immutable{" "}
            <code>source_record</code> with the canonical URL, fetched-at
            timestamp, and content hash. The "Sources & confidence" panel
            on every bill page renders these directly.
          </li>
          <li>
            <strong>Confidence is a first-class field.</strong> Speaker
            attribution and entity matches carry confidence labels rather
            than being presented as certainty.
          </li>
          <li>
            <strong>Manual review is a feature, not a failure.</strong>{" "}
            High-stakes joins (organization → PDC employer ID, speaker →
            legislator) flow through a human-reviewed override file
            before they're treated as confirmed.
          </li>
          <li>
            <strong>Start static, grow dynamic.</strong> The MVP renders
            from precomputed bundles; the Go API serves Postgres directly.
          </li>
        </ol>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          The nightly pipeline
        </h2>
        <p className="text-stone-700">
          Three cooperating ingestions, chained by{" "}
          <code>make daily</code>. All three are idempotent and safe to
          re-run.
        </p>
        <ol className="list-decimal space-y-4 pl-6 text-stone-700">
          <li>
            <strong>ingest-session</strong> — pulls every bill in the
            current biennium from the Washington Legislative Web Services
            (LWS) and stores metadata, sponsors, status timeline, and
            hearing references. ~5,000 bills per session at 5 req/sec,
            ~70 minutes wall-clock.
          </li>
          <li>
            <strong>discover-hearings</strong> — for every LWS-reported
            hearing whose CSI agenda ID and TVW event ID are still blank,
            walks four lookups: CSI committee → CSI meeting (±15-minute
            match) → CSI agenda item (matched by leading bill number) →
            TVW event (committee-name sanity check, ±2-hour window).
            About three quarters of LWS-reported hearings match cleanly;
            the rest are LWS-optimistic non-hearings that get correctly
            rejected.
          </li>
          <li>
            <strong>ingest-hearings</strong> — for every agenda item
            discovery populated whose hearing has a TVW event, runs the
            full pipeline: CSI testifier sign-ins, TVW WebVTT captions,
            transcript segmentation by bill-mention regex, deterministic
            speaker attribution, and PDC lobbying / campaign-finance
            context for reviewed organization matches.
          </li>
        </ol>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          What the source feeds are
        </h2>
        <p className="text-stone-700">
          The{" "}
          <Link href="/sources" className="text-blue-700 underline hover:text-blue-900">
            Sources page
          </Link>{" "}
          lists every official feed with live recorded-call counts and
          the endpoints we actually call. The active families today are
          the Washington Legislative Web Services, the Committee Sign In
          public testimony record, TVW (the public-affairs network) and
          its Invintus video platform, and the Washington Public
          Disclosure Commission via data.wa.gov.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">What this is not</h2>
        <ul className="list-disc space-y-2 pl-6 text-stone-700">
          <li>
            <strong>Not a substitute for the official record.</strong>{" "}
            Every claim links back to its source so you can verify and,
            where appropriate, cite the original.
          </li>
          <li>
            <strong>Not editorial.</strong> The structured graph here
            doesn&apos;t carry analysis or opinion. Money and lobbying
            records appear as <em>context</em>, not proof of causation.
          </li>
          <li>
            <strong>Not complete.</strong> Coverage is bounded by what
            ingestion has reached and what the upstream sources expose.
            Speaker attribution is partial; written testimony access is
            still being negotiated.
          </li>
        </ul>
      </section>
    </article>
  );
}
