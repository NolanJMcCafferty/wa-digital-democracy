import Link from "next/link";
import type { Bundle, Position } from "@/lib/bundle";
import { confidenceLabel, formatMS, tvwDeepLink } from "@/lib/format";
import { legislatorSlug, slugify } from "@/lib/loadBundle";
import { SourcePanel } from "../../bills/[biennium]/[billNumber]/_sections/SourcePanel";
import { StyleSwitcher } from "./StyleSwitcher";
import { formatDateShort, sourceSystems, testimonyCounts, topOrganizations, type StyleConfig } from "./styleData";

export function StyledBillPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  if (style.key === "source-graph") return <SourceGraphPage style={style} bundle={bundle} />;
  if (style.key === "civic-newsroom") return <NewsroomPage style={style} bundle={bundle} />;
  if (style.key === "civic-atlas") return <AtlasPage style={style} bundle={bundle} />;
  if (style.key === "ai-assistant") return <AssistantPage style={style} bundle={bundle} />;
  return <PublicRecordPage style={style} bundle={bundle} />;
}

function Shell({ style, bundle, children, max = "max-w-6xl" }: { style: StyleConfig; bundle: Bundle; children: React.ReactNode; max?: string }) {
  const dark = style.key === "source-graph";
  return (
    <div style={{ background: style.bg, color: style.text }} className={`-mx-6 -my-10 min-h-screen px-6 py-8 ${dark ? "dark" : ""}`}>
      <div className={`mx-auto ${max}`}>
        <StyleSwitcher active={style} bundle={bundle} />
        {children}
      </div>
    </div>
  );
}

function Palette({ style }: { style: StyleConfig }) {
  return (
    <div className="flex items-center gap-1" aria-label="Palette">
      {style.palette.map((c) => <span key={c} title={c} style={{ background: c }} className="h-5 w-5 rounded-full border border-black/10" />)}
    </div>
  );
}

function CountPills({ counts }: { counts: ReturnType<typeof testimonyCounts> }) {
  const rows: Array<[Position, string, string]> = [
    ["Pro", "#2E7D32", "Pro"],
    ["Con", "#B42318", "Con"],
    ["Other", "#6B7280", "Other"],
  ];
  return (
    <div className="flex flex-wrap gap-2">
      {rows.map(([p, color, label]) => (
        <span key={p} className="rounded-full bg-white px-3 py-1 text-sm font-semibold shadow-sm ring-1 ring-black/10" style={{ color }}>
          {label}: {counts[p].toLocaleString()}
        </span>
      ))}
    </div>
  );
}

function SourceChips({ bundle, dark = false }: { bundle: Bundle; dark?: boolean }) {
  return <div className="flex flex-wrap gap-2">{sourceSystems(bundle).map((s) => <span key={s} className={`rounded-full px-2.5 py-1 font-mono text-xs ${dark ? "bg-cyan-400/10 text-cyan-200 ring-1 ring-cyan-400/30" : "bg-slate-100 text-slate-700 ring-1 ring-slate-200"}`}>{s}</span>)}</div>;
}

function SponsorLinks({ bundle, className = "" }: { bundle: Bundle; className?: string }) {
  return <>{(bundle.bill.sponsors ?? []).slice(0, 6).map((s, i) => <span key={`${s.name}-${i}`} className={className}><Link href={`/legislators/${legislatorSlug(s)}`} className="underline decoration-1 underline-offset-2">{s.name}</Link>{s.sponsor_type === "Primary" ? " · primary" : ""}</span>)}</>;
}

function PublicRecordPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  return (
    <Shell style={style} bundle={bundle}>
      <article className="space-y-8">
        <header className="rounded-xl border bg-white p-6 shadow-sm" style={{ borderColor: style.border }}>
          <div className="mb-5 flex flex-wrap items-start justify-between gap-4">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.2em]" style={{ color: style.accent }}>Official-adjacent public record</p>
              <h1 className="mt-2 text-4xl font-bold tracking-tight">{bundle.bill.bill_id}</h1>
              <p className="mt-2 text-xl font-semibold text-slate-800">{bundle.bill.title}</p>
            </div>
            <div className="rounded-lg p-3 text-right text-sm" style={{ background: style.secondary, color: style.muted }}>
              <div className="font-mono">{bundle.bill.biennium}</div>
              <div>Last assembled {formatDateShort(bundle.generated_at)}</div>
            </div>
          </div>
          <p className="max-w-3xl text-lg leading-relaxed text-slate-700">{bundle.bill.description}</p>
          <div className="mt-5 grid gap-3 md:grid-cols-4">
            <Fact label="Current status" value={bundle.status.current || "Unknown"} />
            <Fact label="Committee" value={bundle.hearing.committee_name} />
            <Fact label="Hearing" value={formatDateShort(bundle.hearing.meeting_datetime)} />
            <Fact label="Testimony" value={`${counts.total.toLocaleString()} sign-ins`} />
          </div>
        </header>
        <section className="grid gap-6 lg:grid-cols-[1.1fr_0.9fr]">
          <Card title="Process timeline" style={style}><ol className="space-y-3 border-l pl-5" style={{ borderColor: style.border }}>{(bundle.status.timeline ?? []).slice(0, 8).map((e, i) => <li key={i}><div className="text-xs font-mono text-slate-500">{formatDateShort(e.action_date)}</div><div>{e.history_line}</div></li>)}</ol></Card>
          <Card title="Source-backed hearing evidence" style={style}><p className="text-sm text-slate-600">{bundle.hearing.committee_name} · {formatDateShort(bundle.hearing.meeting_datetime)}</p><div className="mt-4"><CountPills counts={counts} /></div><div className="mt-5"><SourceChips bundle={bundle} /></div>{bundle.hearing.tvw_url ? <a className="mt-4 inline-block text-sm font-semibold underline" style={{ color: style.accent }} href={bundle.hearing.tvw_url}>Watch on TVW →</a> : null}</Card>
        </section>
        <TestimonyPreview bundle={bundle} accent={style.accent} />
        <SourcePanel sources={bundle.sources} knownLimitations={bundle.known_limitations} generatedAt={bundle.generated_at} />
      </article>
    </Shell>
  );
}

function NewsroomPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  const segments = bundle.transcript.segments ?? [];
  return (
    <Shell style={style} bundle={bundle} max="max-w-5xl">
      <article className="space-y-10">
        <header className="border-b pb-8" style={{ borderColor: style.border }}>
          <div className="mb-4 flex items-center justify-between gap-4"><p className="font-mono text-xs uppercase tracking-[0.18em]" style={{ color: style.accent }}>Civic Newsroom · Bill brief</p><Palette style={style} /></div>
          <h1 className="font-serif text-5xl font-bold leading-tight tracking-tight">{bundle.bill.bill_id} would address {lowerFirst(bundle.bill.title || "a Washington policy issue")}</h1>
          <p className="mt-4 max-w-3xl text-xl leading-relaxed" style={{ color: style.muted }}>At a {formatDateShort(bundle.hearing.meeting_datetime)} hearing, Washington lawmakers heard public testimony on {bundle.bill.description?.replace(/\.$/, "") || "the measure"}. Here is the plain-English brief — with the source trail attached.</p>
          <div className="mt-6"><CountPills counts={counts} /></div>
        </header>
        <section className="grid gap-5 md:grid-cols-3">
          <NewsFact label="Status" value={bundle.status.current || "Unknown"} />
          <NewsFact label="Committee" value={bundle.hearing.committee_name} />
          <NewsFact label="Sources" value={sourceSystems(bundle).join(" · ")} />
        </section>
        <section className="grid gap-8 lg:grid-cols-[1fr_320px]">
          <div className="space-y-6 text-lg leading-relaxed">
            <h2 className="font-serif text-3xl font-bold">What this bill would do</h2>
            <p>{bundle.bill.description}</p>
            <p className="rounded-xl border-l-4 bg-white p-5 text-base shadow-sm" style={{ borderColor: style.accent }}>Summary generated from official source metadata; not official legislative analysis.</p>
            <h2 className="font-serif text-3xl font-bold">What happened in the hearing</h2>
            <p>The hearing record shows {counts.Pro.toLocaleString()} pro sign-ins, {counts.Con.toLocaleString()} con sign-ins, and {counts.Other.toLocaleString()} other positions. {counts.testified.toLocaleString()} people testified live or were captured as testifying in CSI.</p>
            {segments[0] ? <blockquote className="border-l-4 bg-white p-5 text-xl italic shadow-sm" style={{ borderColor: "#C2410C" }}>“{segments[0].text.slice(0, 240)}{segments[0].text.length > 240 ? "…" : ""}”<footer className="mt-3 font-mono text-xs not-italic" style={{ color: style.muted }}>{segments[0].speaker_label || "Transcript segment"} · {formatMS(segments[0].start_ms)}</footer></blockquote> : null}
          </div>
          <aside className="space-y-4"><Card title="Reporter notebook" style={style}><p className="text-sm">Top linked organizations:</p><ul className="mt-2 list-disc pl-5 text-sm">{topOrganizations(bundle, 5).map(o => <li key={o}>{o}</li>)}</ul></Card><Card title="Receipts" style={style}><SourceChips bundle={bundle} /><p className="mt-3 text-xs" style={{ color: style.muted }}>Every narrative claim should open into a CSI row, TVW timestamp, LWS record, or PDC context row.</p></Card></aside>
        </section>
        <TestimonyPreview bundle={bundle} accent={style.accent} />
      </article>
    </Shell>
  );
}

function SourceGraphPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  const darkCard = "rounded-xl border border-slate-700 bg-slate-900/70 p-5 shadow-2xl";
  return (
    <Shell style={style} bundle={bundle} max="max-w-7xl">
      <article className="grid gap-5 lg:grid-cols-[220px_1fr_340px]">
        <aside className="space-y-3 rounded-xl border border-slate-700 bg-slate-900/80 p-4 text-sm text-slate-300"><p className="text-xs font-semibold uppercase tracking-[0.18em] text-cyan-300">Graph nav</p>{["Bill", "Hearing", "Testifiers", "Organizations", "Transcript", "Money", "Sources"].map(x => <div key={x} className="rounded-lg bg-slate-800 px-3 py-2">{x}</div>)}</aside>
        <main className="space-y-5">
          <header className={darkCard}><p className="text-xs font-mono uppercase tracking-[0.18em] text-cyan-300">Entity · Bill</p><h1 className="mt-2 text-4xl font-bold text-white">{bundle.bill.bill_id}</h1><p className="mt-2 text-xl text-slate-200">{bundle.bill.title}</p><p className="mt-4 max-w-3xl text-slate-400">{style.oneLine}</p></header>
          <section className={darkCard}><h2 className="mb-4 text-xl font-semibold text-white">Relationship map</h2><div className="space-y-3 font-mono text-sm"><GraphLine a={bundle.bill.bill_id} b={`Public hearing · ${bundle.hearing.committee_acronym || bundle.hearing.committee_name}`} /><GraphLine a="Hearing" b={`${counts.total.toLocaleString()} CSI sign-ins`} /><GraphLine a="Testimony" b={`${topOrganizations(bundle, 1)[0] || "Organizations"} + related PDC context`} /><GraphLine a="Transcript" b={`${(bundle.transcript.segments ?? []).length.toLocaleString()} timestamped segments`} /></div></section>
          <section className="grid gap-4 md:grid-cols-3"><Metric label="Pro" value={counts.Pro} color="#4ADE80" /><Metric label="Con" value={counts.Con} color="#F87171" /><Metric label="Other" value={counts.Other} color="#9CA3AF" /></section>
          <section className={darkCard}><h2 className="mb-3 text-xl font-semibold text-white">Evidence panes</h2><TestimonyMiniDark bundle={bundle} /></section>
        </main>
        <aside className="space-y-4"><div className={darkCard}><h2 className="text-lg font-semibold text-white">Inspector</h2><dl className="mt-4 space-y-3 text-sm"><DarkFact label="Status" value={bundle.status.current || "Unknown"} /><DarkFact label="Hearing date" value={formatDateShort(bundle.hearing.meeting_datetime)} /><DarkFact label="Committee" value={bundle.hearing.committee_name} /><DarkFact label="Match confidence" value="Visible per entity" /></dl></div><div className={darkCard}><h2 className="mb-3 text-lg font-semibold text-white">Source records</h2><SourceChips bundle={bundle} dark /></div></aside>
      </article>
    </Shell>
  );
}

function AtlasPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  return (
    <Shell style={style} bundle={bundle}>
      <article className="space-y-8">
        <header className="overflow-hidden rounded-3xl border bg-white shadow-sm" style={{ borderColor: style.border }}><div className="grid lg:grid-cols-[1fr_360px]"><div className="p-8"><p className="text-xs font-bold uppercase tracking-[0.2em]" style={{ color: style.accent }}>Washington Civic Atlas</p><h1 className="mt-3 font-serif text-5xl font-bold tracking-tight">{bundle.bill.bill_id}: {bundle.bill.title}</h1><p className="mt-4 max-w-3xl text-lg leading-relaxed" style={{ color: style.muted }}>{bundle.bill.description}</p><div className="mt-6 flex flex-wrap gap-2"><SponsorLinks bundle={bundle} className="rounded-full px-3 py-1 text-sm ring-1" /></div></div><div className="relative min-h-72 border-l p-6" style={{ background: style.secondary, borderColor: style.border }}><div className="absolute inset-0 opacity-30" style={{ backgroundImage: "radial-gradient(circle at 1px 1px, #1F5C45 1px, transparent 0)", backgroundSize: "22px 22px" }} /><div className="relative rounded-2xl border bg-white/80 p-5 shadow-sm" style={{ borderColor: style.border }}><p className="font-serif text-2xl font-bold">Local relevance panel</p><p className="mt-2 text-sm" style={{ color: style.muted }}>Sponsor districts, committee member districts, agency jurisdictions, and local overlays would live here as data coverage expands.</p></div></div></div></header>
        <section className="grid gap-5 md:grid-cols-4"><AtlasFact label="Hearing" value={formatDateShort(bundle.hearing.meeting_datetime)} /><AtlasFact label="Committee" value={bundle.hearing.committee_acronym || bundle.hearing.committee_name} /><AtlasFact label="Pro / Con" value={`${counts.Pro} / ${counts.Con}`} /><AtlasFact label="Sources" value={sourceSystems(bundle).length.toString()} /></section>
        <section className="grid gap-6 lg:grid-cols-[1fr_0.9fr]"><Card title="Issue and community context" style={style}><p className="leading-relaxed">This style makes the same bill page feel native to Washington: committee records plus future district, county, city, housing, transportation, and agency overlays.</p><div className="mt-4 grid grid-cols-2 gap-3 text-sm"><div className="rounded-xl p-3" style={{ background: style.secondary }}>District lookup slot</div><div className="rounded-xl p-3" style={{ background: style.secondary }}>County/city mentions</div><div className="rounded-xl p-3" style={{ background: style.secondary }}>Agency impact</div><div className="rounded-xl p-3" style={{ background: style.secondary }}>Issue overlays</div></div></Card><Card title="Hearing/testimony" style={style}><CountPills counts={counts} /><div className="mt-4"><TestimonyMini bundle={bundle} /></div></Card></section>
        <SourcePanel sources={bundle.sources} knownLimitations={bundle.known_limitations} generatedAt={bundle.generated_at} />
      </article>
    </Shell>
  );
}

function AssistantPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  return (
    <Shell style={style} bundle={bundle} max="max-w-5xl">
      <article className="space-y-8">
        <header className="rounded-[2rem] border bg-white p-8 shadow-sm" style={{ borderColor: style.border }}><p className="text-center text-xs font-semibold uppercase tracking-[0.2em]" style={{ color: style.accent }}>Transparent AI Civic Assistant</p><h1 className="mt-3 text-center text-4xl font-bold tracking-tight">Ask about {bundle.bill.bill_id}</h1><div className="mx-auto mt-6 max-w-3xl rounded-2xl border bg-slate-50 p-4" style={{ borderColor: style.border }}><p className="text-lg text-slate-500">Who testified for and against this bill, and what sources support that answer?</p><div className="mt-4 flex flex-wrap gap-2 text-sm"><Prompt>Compare pro vs con arguments</Prompt><Prompt>Show only official sources</Prompt><Prompt>Open transcript evidence</Prompt></div></div></header>
        <section className="rounded-3xl border bg-white p-6 shadow-sm" style={{ borderColor: style.border }}><div className="mb-4 flex items-start justify-between gap-4"><div><p className="text-xs font-bold uppercase tracking-[0.18em]" style={{ color: style.accent }}>Precomputed answer</p><h2 className="mt-1 text-2xl font-bold">What the public record says</h2></div><span className="rounded-full px-3 py-1 text-xs font-semibold" style={{ background: style.secondary, color: style.accent }}>Confidence: high for sign-in counts</span></div><p className="text-lg leading-relaxed">{bundle.bill.bill_id} concerns {lowerFirst(bundle.bill.title || "this bill")}. The current status is: <strong>{bundle.status.current || "unknown"}</strong>. The hearing record includes <strong>{counts.Pro.toLocaleString()} pro</strong>, <strong>{counts.Con.toLocaleString()} con</strong>, and <strong>{counts.Other.toLocaleString()} other</strong> sign-ins.</p><div className="mt-5"><SourceChips bundle={bundle} /></div></section>
        <section className="grid gap-5 md:grid-cols-2"><Card title="Evidence cards" style={style}><ul className="space-y-3 text-sm"><li>CSI sign-in rows for positions and organizations</li><li>LWS bill metadata, sponsors, and status history</li><li>TVW/Invintus transcript segments and video timestamps</li><li>PDC context where reviewed organization matches exist</li></ul></Card><Card title="Bill brief" style={style}><p>{bundle.bill.description}</p><div className="mt-4"><CountPills counts={counts} /></div></Card></section>
        <TestimonyPreview bundle={bundle} accent={style.accent} />
      </article>
    </Shell>
  );
}

function Fact({ label, value }: { label: string; value: string }) { return <div className="rounded-lg border border-slate-200 bg-slate-50 p-3"><dt className="text-xs font-semibold uppercase tracking-wider text-slate-500">{label}</dt><dd className="mt-1 text-sm font-medium text-slate-900">{value}</dd></div>; }
function NewsFact({ label, value }: { label: string; value: string }) { return <div className="rounded-xl border bg-white p-4 shadow-sm"><p className="font-mono text-xs uppercase tracking-wider text-stone-500">{label}</p><p className="mt-2 font-serif text-xl font-bold">{value}</p></div>; }
function AtlasFact({ label, value }: { label: string; value: string }) { return <div className="rounded-2xl border bg-white p-4 shadow-sm"><p className="text-xs font-bold uppercase tracking-wider text-slate-500">{label}</p><p className="mt-2 text-xl font-semibold">{value}</p></div>; }
function Card({ title, style, children }: { title: string; style: StyleConfig; children: React.ReactNode }) { return <section className="rounded-xl border bg-white p-5 shadow-sm" style={{ borderColor: style.border }}><h2 className="mb-3 text-xl font-semibold" style={{ color: style.text }}>{title}</h2>{children}</section>; }
function Prompt({ children }: { children: React.ReactNode }) { return <span className="rounded-full bg-white px-3 py-1 text-indigo-700 ring-1 ring-indigo-100">{children}</span>; }
function Metric({ label, value, color }: { label: string; value: number; color: string }) { return <div className="rounded-xl border border-slate-700 bg-slate-900 p-5"><p className="text-sm text-slate-400">{label}</p><p className="mt-1 text-3xl font-bold" style={{ color }}>{value.toLocaleString()}</p></div>; }
function DarkFact({ label, value }: { label: string; value: string }) { return <div><dt className="text-xs uppercase tracking-wider text-slate-500">{label}</dt><dd className="text-slate-200">{value}</dd></div>; }
function GraphLine({ a, b }: { a: string; b: string }) { return <div className="flex flex-wrap items-center gap-2"><span className="rounded bg-slate-800 px-2 py-1 text-cyan-200">{a}</span><span className="text-slate-500">→</span><span className="rounded bg-slate-800 px-2 py-1 text-emerald-200">{b}</span></div>; }
function TestimonyMini({ bundle }: { bundle: Bundle }) { return <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white text-sm">{(bundle.testifiers ?? []).filter(t => t.testified).slice(0, 5).map((t, i) => <li key={i} className="p-3"><strong>{t.raw_name}</strong>{t.raw_organization ? ` · ${t.raw_organization}` : ""}<span className="ml-2 text-slate-500">{t.position}</span></li>)}</ul>; }
function TestimonyMiniDark({ bundle }: { bundle: Bundle }) { return <ul className="divide-y divide-slate-800 text-sm">{(bundle.testifiers ?? []).filter(t => t.testified).slice(0, 6).map((t, i) => <li key={i} className="py-2 text-slate-300"><span className="text-white">{t.raw_name}</span>{t.raw_organization ? ` · ${t.raw_organization}` : ""}<span className="ml-2 text-cyan-300">{t.position}</span></li>)}</ul>; }
function TestimonyPreview({ bundle, accent }: { bundle: Bundle; accent: string }) { const segs = bundle.transcript.segments ?? []; return <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="mb-3 text-xl font-semibold text-slate-900">Timestamped evidence preview</h2>{segs.length === 0 ? <p className="text-sm text-slate-600">No matched transcript segments available.</p> : <ol className="space-y-3">{segs.slice(0, 3).map((s, i) => { const href = bundle.hearing.tvw_event_id ? tvwDeepLink(bundle.hearing.tvw_event_id, s.start_ms) : bundle.transcript.caption_url || "#"; return <li key={i} className="rounded-lg bg-slate-50 p-4"><a href={href} className="font-mono text-xs underline" style={{ color: accent }}>{formatMS(s.start_ms)}</a><span className="ml-2 text-xs text-slate-500">{s.speaker_label || "unknown speaker"} · {confidenceLabel(s.speaker_confidence)}</span><p className="mt-2 text-slate-800">{s.text.slice(0, 260)}{s.text.length > 260 ? "…" : ""}</p></li>; })}</ol>}</section>; }
function lowerFirst(s: string) { return s ? s.charAt(0).toLowerCase() + s.slice(1) : s; }
