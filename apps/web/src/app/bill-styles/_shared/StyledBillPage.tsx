import Link from "next/link";
import type { Bundle, Position, Testifier, TranscriptSegment } from "@/lib/bundle";
import { confidenceLabel, formatDateTime, formatMS, tvwDeepLink } from "@/lib/format";
import { legislatorSlug, slugify } from "@/lib/loadBundle";
import { StyleSwitcher } from "./StyleSwitcher";
import {
  formatDateShort,
  sourceSystems,
  testimonyCounts,
  topOrganizations,
  type StyleConfig,
} from "./styleData";

export function StyledBillPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  if (style.key === "testimony-theatre") return <TestimonyTheatrePage style={style} bundle={bundle} />;
  if (style.key === "power-map") return <PowerMapPage style={style} bundle={bundle} />;
  if (style.key === "civic-docket") return <CivicDocketPage style={style} bundle={bundle} />;
  if (style.key === "signal-desk") return <SignalDeskPage style={style} bundle={bundle} />;
  return <DecisionBriefPage style={style} bundle={bundle} />;
}

function Shell({
  style,
  bundle,
  children,
  max = "max-w-7xl",
}: {
  style: StyleConfig;
  bundle: Bundle;
  children: React.ReactNode;
  max?: string;
}) {
  const isDark = style.bg.startsWith("#0") || style.bg.startsWith("#1");
  return (
    <div
      style={{ background: style.bg, color: style.text }}
      className={`-mx-6 -my-10 min-h-screen px-4 py-6 sm:px-6 sm:py-8 ${isDark ? "dark" : ""}`}
    >
      <div className={`mx-auto ${max}`}>
        <StyleSwitcher active={style} bundle={bundle} />
        {children}
      </div>
    </div>
  );
}

function DecisionBriefPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  const latest = bundle.status.timeline?.at(-1);
  const topOrgs = topOrganizations(bundle, 4);
  return (
    <Shell style={style} bundle={bundle} max="max-w-6xl">
      <article className="space-y-8">
        <header className="grid gap-6 lg:grid-cols-[1fr_360px]">
          <div className="rounded-[2rem] border bg-white p-8 shadow-sm" style={{ borderColor: style.border }}>
            <p className="text-xs font-black uppercase tracking-[0.24em]" style={{ color: style.accent }}>
              Decision brief
            </p>
            <h1 className="mt-4 max-w-4xl font-serif text-5xl font-black leading-[0.95] tracking-tight text-stone-950 sm:text-6xl">
              {bundle.bill.bill_id}: {bundle.bill.title}
            </h1>
            <p className="mt-6 max-w-3xl text-xl leading-8 text-stone-700">{bundle.bill.description}</p>
            <div className="mt-8 flex flex-wrap gap-2">
              <Chip tone="amber">{bundle.bill.biennium}</Chip>
              <Chip tone="blue">{bundle.hearing.committee_name}</Chip>
              <Chip tone="green">{counts.total.toLocaleString()} public positions</Chip>
            </div>
          </div>
          <aside className="rounded-[2rem] border p-6 shadow-sm" style={{ borderColor: style.border, background: style.secondary }}>
            <p className="text-xs font-black uppercase tracking-[0.2em] text-stone-500">Read this first</p>
            <dl className="mt-5 space-y-5">
              <BriefFact label="Current status" value={bundle.status.current || "Status unavailable"} />
              <BriefFact label="Most recent movement" value={latest?.history_line || "No timeline entry available"} />
              <BriefFact label="Hearing" value={formatDateTime(bundle.hearing.meeting_datetime)} />
            </dl>
          </aside>
        </header>

        <section className="grid gap-4 md:grid-cols-4">
          <BigNumber label="Pro" value={counts.Pro} color="#0F766E" />
          <BigNumber label="Con" value={counts.Con} color="#B42318" />
          <BigNumber label="Other" value={counts.Other} color="#6B7280" />
          <BigNumber label="Testified" value={counts.testified} color="#B45309" />
        </section>

        <section className="grid gap-6 lg:grid-cols-[1fr_1fr]">
          <Panel title="What matters" style={style}>
            <ul className="space-y-4 text-base leading-7 text-stone-700">
              <li><strong className="text-stone-950">The policy object:</strong> {bundle.bill.description}</li>
              <li><strong className="text-stone-950">The public record:</strong> testimony is sharply weighted {counts.Con > counts.Pro ? "against" : counts.Pro > counts.Con ? "in favor of" : "across positions on"} the bill in this hearing record.</li>
              <li><strong className="text-stone-950">The next click:</strong> inspect the testimony and transcript excerpts before treating any summary as complete.</li>
            </ul>
          </Panel>
          <Panel title="Stakeholders to inspect" style={style}>
            {topOrgs.length > 0 ? (
              <ul className="space-y-3">
                {topOrgs.map((org) => (
                  <li key={org}>
                    <Link href={`/organizations/${slugify(org)}`} className="group flex items-center justify-between rounded-2xl border border-stone-200 bg-stone-50 px-4 py-3 text-stone-900 hover:bg-white">
                      <span className="font-semibold">{org}</span>
                      <span className="text-sm text-stone-500 group-hover:text-stone-900">Open →</span>
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-stone-600">No reviewed organization links are available yet.</p>
            )}
          </Panel>
        </section>

        <TranscriptCards bundle={bundle} accent={style.accent} />
        <ReceiptsPanel bundle={bundle} style={style} />
      </article>
    </Shell>
  );
}

function TestimonyTheatrePage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  const segments = bundle.transcript.segments ?? [];
  const quote = bestQuote(segments);
  return (
    <Shell style={style} bundle={bundle} max="max-w-6xl">
      <article className="space-y-8">
        <header className="relative overflow-hidden rounded-[2.5rem] border p-8 shadow-2xl" style={{ borderColor: style.border, background: `radial-gradient(circle at 20% 10%, #5B2A86 0, transparent 34%), linear-gradient(135deg, ${style.surface}, ${style.bg})` }}>
          <div className="absolute right-8 top-8 hidden h-40 w-40 rounded-full bg-orange-300/20 blur-3xl md:block" />
          <p className="text-xs font-black uppercase tracking-[0.28em]" style={{ color: style.accent }}>Testimony theatre</p>
          <h1 className="mt-5 max-w-4xl text-5xl font-black leading-none tracking-tight text-[#FFF5E8] sm:text-7xl">
            Hear the bill through the room.
          </h1>
          <p className="mt-6 max-w-3xl text-xl leading-8 text-[#D9CBE6]">
            {bundle.bill.bill_id} — {bundle.bill.title}. A hearing-centered design that treats testimony as the primary civic artifact, not a table afterthought.
          </p>
          <div className="mt-8 grid gap-3 sm:grid-cols-3">
            <TheatreStat label="For" value={counts.Pro} />
            <TheatreStat label="Against" value={counts.Con} />
            <TheatreStat label="On the record" value={counts.total} />
          </div>
        </header>

        <section className="grid gap-6 lg:grid-cols-[1fr_340px]">
          <div className="rounded-[2rem] border p-6" style={{ borderColor: style.border, background: style.surface }}>
            <div className="mb-4 flex items-center justify-between gap-4">
              <h2 className="text-2xl font-black text-[#FFF5E8]">Featured timestamp</h2>
              {quote ? <TimestampLink bundle={bundle} segment={quote} className="text-sm font-bold text-orange-200 underline" /> : null}
            </div>
            {quote ? (
              <blockquote className="text-3xl font-semibold leading-tight text-[#FFF5E8]">
                “{quote.text.slice(0, 360)}{quote.text.length > 360 ? "…" : ""}”
                <footer className="mt-5 text-sm font-normal uppercase tracking-[0.18em] text-[#C7B8D8]">
                  {quote.speaker_label || "Transcript segment"} · {confidenceLabel(quote.speaker_confidence)}
                </footer>
              </blockquote>
            ) : (
              <p className="text-[#C7B8D8]">No transcript segments are available yet.</p>
            )}
          </div>
          <aside className="rounded-[2rem] border p-6" style={{ borderColor: style.border, background: style.secondary }}>
            <h2 className="text-xl font-black text-[#FFF5E8]">Scene card</h2>
            <dl className="mt-5 space-y-4 text-sm">
              <DarkRow label="Committee" value={bundle.hearing.committee_name} />
              <DarkRow label="Date" value={formatDateTime(bundle.hearing.meeting_datetime)} />
              <DarkRow label="Agenda" value={bundle.hearing.agenda_item_label || bundle.bill.bill_id} />
              <DarkRow label="Video" value={bundle.hearing.tvw_url ? "TVW available" : "No TVW link"} />
            </dl>
          </aside>
        </section>

        <section className="grid gap-4 md:grid-cols-3">
          <TestifierColumn title="Support" items={byPosition(bundle.testifiers, "Pro")} color="#4ADE80" />
          <TestifierColumn title="Opposition" items={byPosition(bundle.testifiers, "Con")} color="#FB7185" />
          <TestifierColumn title="Other" items={byPosition(bundle.testifiers, "Other")} color="#A78BFA" />
        </section>
        <TranscriptCards bundle={bundle} accent={style.accent} dark />
      </article>
    </Shell>
  );
}

function PowerMapPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  const orgs = bundle.organizations ?? [];
  return (
    <Shell style={style} bundle={bundle}>
      <article className="space-y-8">
        <header className="rounded-[2rem] border bg-white p-8 shadow-sm" style={{ borderColor: style.border }}>
          <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <p className="text-xs font-black uppercase tracking-[0.24em]" style={{ color: style.accent }}>Power map</p>
              <h1 className="mt-3 max-w-4xl text-5xl font-black tracking-tight text-slate-950">Who is lining up around {bundle.bill.bill_id}?</h1>
              <p className="mt-4 max-w-3xl text-lg leading-8 text-slate-600">A stakeholder-first page that makes relationships, pressure, and source confidence visible before the user dives into raw records.</p>
            </div>
            <div className="grid grid-cols-3 gap-2 text-center">
              <MiniGauge label="Pro" value={counts.Pro} />
              <MiniGauge label="Con" value={counts.Con} />
              <MiniGauge label="Other" value={counts.Other} />
            </div>
          </div>
        </header>

        <section className="grid gap-6 lg:grid-cols-[320px_1fr_320px]">
          <MapRail title="Bill" items={[bundle.bill.bill_id, bundle.bill.title || "Untitled", bundle.status.current || "Status unavailable"]} accent={style.accent} />
          <div className="rounded-[2rem] border bg-white p-6 shadow-sm" style={{ borderColor: style.border }}>
            <h2 className="text-2xl font-black text-slate-950">Stakeholder field</h2>
            <div className="mt-6 grid gap-4 sm:grid-cols-2">
              {(orgs.length ? orgs : placeholderOrganizations()).slice(0, 8).map((org, i) => (
                <div key={org.canonical_name} className="rounded-3xl border p-5" style={{ borderColor: i % 2 ? "#C7D2FE" : "#BBF7D0", background: i % 2 ? "#F5F3FF" : "#F0FDF4" }}>
                  <p className="text-sm font-black uppercase tracking-[0.16em] text-slate-500">{org.testifier_position || (i % 2 ? "Context" : "Public record")}</p>
                  <h3 className="mt-2 text-lg font-black text-slate-950">{org.canonical_name}</h3>
                  <p className="mt-3 text-sm text-slate-600">{org.testifier_count ?? 0} linked testifier{(org.testifier_count ?? 0) === 1 ? "" : "s"} · {org.match_confidence}</p>
                </div>
              ))}
            </div>
          </div>
          <MapRail title="Receipts" items={sourceSystems(bundle).map((s) => s.toUpperCase())} accent="#12B76A" />
        </section>

        <section className="grid gap-6 lg:grid-cols-[1fr_1fr]">
          <Panel title="Sponsorship" style={style}>
            <div className="flex flex-wrap gap-2">
              {(bundle.bill.sponsors ?? []).slice(0, 12).map((s) => (
                <Link key={`${s.name}-${s.sponsor_type}`} href={`/legislators/${legislatorSlug(s)}`} className="rounded-full bg-violet-50 px-3 py-1.5 text-sm font-semibold text-violet-900 ring-1 ring-violet-200">
                  {s.name}{s.sponsor_type === "Primary" ? " · primary" : ""}
                </Link>
              ))}
            </div>
          </Panel>
          <Panel title="Public testimony shape" style={style}>
            <StackedBar counts={counts} />
          </Panel>
        </section>
      </article>
    </Shell>
  );
}

function CivicDocketPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  return (
    <Shell style={style} bundle={bundle} max="max-w-6xl">
      <article className="grid gap-8 lg:grid-cols-[280px_1fr]">
        <aside className="lg:sticky lg:top-6 lg:self-start">
          <div className="rounded-sm border bg-white p-5 shadow-sm" style={{ borderColor: style.border }}>
            <p className="text-xs font-bold uppercase tracking-[0.22em] text-slate-500">Case file</p>
            <h1 className="mt-3 text-3xl font-black text-slate-950">{bundle.bill.bill_id}</h1>
            <p className="mt-2 text-sm text-slate-600">{bundle.bill.title}</p>
            <div className="my-5 border-t" style={{ borderColor: style.border }} />
            <nav className="space-y-2 text-sm font-semibold text-blue-800">
              {['Summary', 'Docket', 'Hearing', 'Testimony', 'Sources'].map((x) => <a key={x} href={`#${x.toLowerCase()}`} className="block hover:underline">{x}</a>)}
            </nav>
          </div>
        </aside>
        <main className="space-y-6">
          <section id="summary" className="rounded-sm border bg-white p-8 shadow-sm" style={{ borderColor: style.border }}>
            <p className="text-xs font-bold uppercase tracking-[0.24em]" style={{ color: style.accent }}>Civic docket</p>
            <h2 className="mt-3 font-serif text-5xl font-black tracking-tight text-slate-950">{bundle.bill.title}</h2>
            <p className="mt-5 text-lg leading-8 text-slate-700">{bundle.bill.description}</p>
            <dl className="mt-8 grid gap-3 sm:grid-cols-2">
              <DocketFact label="Current status" value={bundle.status.current || "Unknown"} />
              <DocketFact label="Hearing" value={formatDateTime(bundle.hearing.meeting_datetime)} />
              <DocketFact label="Committee" value={bundle.hearing.committee_name} />
              <DocketFact label="Public positions" value={`${counts.total.toLocaleString()} sign-ins`} />
            </dl>
          </section>
          <section id="docket" className="rounded-sm border bg-white p-8 shadow-sm" style={{ borderColor: style.border }}>
            <h2 className="text-2xl font-black text-slate-950">Docket history</h2>
            <ol className="mt-6 space-y-0 border-l-2 border-slate-300">
              {(bundle.status.timeline ?? []).map((e, i) => (
                <li key={`${e.action_date}-${i}`} className="relative pb-5 pl-6">
                  <span className="absolute -left-[7px] top-1 h-3 w-3 rounded-full bg-blue-700 ring-4 ring-white" />
                  <p className="font-mono text-xs text-slate-500">{formatDateShort(e.action_date)}</p>
                  <p className="mt-1 text-slate-800">{e.history_line}</p>
                </li>
              ))}
            </ol>
          </section>
          <section id="testimony" className="rounded-sm border bg-white p-8 shadow-sm" style={{ borderColor: style.border }}>
            <h2 className="text-2xl font-black text-slate-950">Testimony record</h2>
            <div className="mt-5"><StackedBar counts={counts} /></div>
            <TestifierTableLite testifiers={bundle.testifiers} />
          </section>
          <ReceiptsPanel bundle={bundle} style={style} />
        </main>
      </article>
    </Shell>
  );
}

function SignalDeskPage({ style, bundle }: { style: StyleConfig; bundle: Bundle }) {
  const counts = testimonyCounts(bundle);
  const segments = bundle.transcript.segments ?? [];
  const conShare = counts.total ? Math.round((counts.Con / counts.total) * 100) : 0;
  return (
    <Shell style={style} bundle={bundle} max="max-w-7xl">
      <article className="space-y-5">
        <header className="grid gap-5 lg:grid-cols-[1fr_420px]">
          <div className="rounded-3xl border p-6" style={{ borderColor: style.border, background: style.surface }}>
            <p className="font-mono text-xs font-bold uppercase tracking-[0.24em] text-cyan-300">Signal desk</p>
            <h1 className="mt-4 text-5xl font-black leading-none tracking-tight text-cyan-50">{bundle.bill.bill_id} / {bundle.bill.title}</h1>
            <p className="mt-4 max-w-3xl text-lg leading-7 text-slate-300">An operator view for quickly spotting movement, imbalance, source coverage, and transcript leads.</p>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <SignalMetric label="Opposition share" value={`${conShare}%`} hot={conShare > 50} />
            <SignalMetric label="Transcript hits" value={segments.length.toString()} />
            <SignalMetric label="Sources" value={sourceSystems(bundle).length.toString()} />
            <SignalMetric label="Testified" value={counts.testified.toString()} />
          </div>
        </header>

        <section className="grid gap-5 lg:grid-cols-[300px_1fr_360px]">
          <div className="space-y-5">
            <SignalCard title="Current status" value={bundle.status.current || "Unknown"} />
            <SignalCard title="Committee" value={bundle.hearing.committee_name} />
            <SignalCard title="Hearing date" value={formatDateShort(bundle.hearing.meeting_datetime)} />
          </div>
          <div className="rounded-3xl border p-5" style={{ borderColor: style.border, background: style.surface }}>
            <h2 className="text-xl font-black text-cyan-50">Signal stream</h2>
            <ol className="mt-5 space-y-3">
              {signalItems(bundle).map((item) => (
                <li key={item} className="rounded-2xl border border-cyan-400/20 bg-cyan-400/5 p-4 text-sm text-slate-200">
                  {item}
                </li>
              ))}
            </ol>
          </div>
          <div className="rounded-3xl border p-5" style={{ borderColor: style.border, background: style.surface }}>
            <h2 className="text-xl font-black text-cyan-50">Transcript leads</h2>
            <div className="mt-4 space-y-3">
              {segments.slice(0, 5).map((s) => (
                <div key={s.start_ms} className="rounded-2xl bg-slate-950/40 p-4">
                  <TimestampLink bundle={bundle} segment={s} className="font-mono text-xs font-bold text-cyan-300 underline" />
                  <p className="mt-2 text-sm text-slate-300">{s.text.slice(0, 160)}{s.text.length > 160 ? "…" : ""}</p>
                </div>
              ))}
            </div>
          </div>
        </section>
      </article>
    </Shell>
  );
}

function BriefFact({ label, value }: { label: string; value: string }) {
  return <div><dt className="text-xs font-bold uppercase tracking-[0.18em] text-stone-500">{label}</dt><dd className="mt-1 text-sm font-semibold leading-6 text-stone-900">{value}</dd></div>;
}

function BigNumber({ label, value, color }: { label: string; value: number; color: string }) {
  return <div className="rounded-[1.5rem] border border-stone-200 bg-white p-5 shadow-sm"><p className="text-sm font-bold text-stone-500">{label}</p><p className="mt-2 text-5xl font-black" style={{ color }}>{value.toLocaleString()}</p></div>;
}

function Panel({ title, style, children }: { title: string; style: StyleConfig; children: React.ReactNode }) {
  return <section className="rounded-[1.5rem] border bg-white p-6 shadow-sm" style={{ borderColor: style.border }}><h2 className="mb-4 text-2xl font-black" style={{ color: style.text }}>{title}</h2>{children}</section>;
}

function Chip({ children, tone }: { children: React.ReactNode; tone: "amber" | "blue" | "green" }) {
  const cls = tone === "amber" ? "bg-amber-100 text-amber-900" : tone === "blue" ? "bg-blue-100 text-blue-900" : "bg-emerald-100 text-emerald-900";
  return <span className={`${cls} rounded-full px-3 py-1.5 text-sm font-bold`}>{children}</span>;
}

function TheatreStat({ label, value }: { label: string; value: number }) {
  return <div className="rounded-2xl border border-white/10 bg-white/10 p-4"><p className="text-xs font-bold uppercase tracking-[0.18em] text-[#C7B8D8]">{label}</p><p className="mt-2 text-4xl font-black text-[#FFF5E8]">{value.toLocaleString()}</p></div>;
}

function DarkRow({ label, value }: { label: string; value: string }) {
  return <div><dt className="text-xs uppercase tracking-[0.18em] text-[#A995C0]">{label}</dt><dd className="mt-1 font-semibold text-[#FFF5E8]">{value}</dd></div>;
}

function TestifierColumn({ title, items, color }: { title: string; items: Testifier[]; color: string }) {
  return <section className="rounded-[2rem] border border-white/10 bg-white/5 p-5"><h2 className="text-xl font-black" style={{ color }}>{title}</h2><ul className="mt-4 space-y-3">{items.slice(0, 8).map((t) => <li key={`${t.raw_name}-${t.raw_organization}`} className="rounded-2xl bg-black/20 p-3 text-sm text-[#F8EEDF]"><strong>{t.raw_name}</strong>{t.raw_organization ? <span className="block text-[#C7B8D8]">{t.raw_organization}</span> : null}</li>)}</ul></section>;
}

function MapRail({ title, items, accent }: { title: string; items: string[]; accent: string }) {
  return <aside className="rounded-[2rem] border bg-white p-5 shadow-sm" style={{ borderColor: "#E4E7EC" }}><h2 className="text-lg font-black text-slate-950">{title}</h2><ul className="mt-4 space-y-3">{items.slice(0, 8).map((item) => <li key={item} className="rounded-2xl px-4 py-3 text-sm font-semibold" style={{ background: `${accent}14`, color: "#101828" }}>{item}</li>)}</ul></aside>;
}

function MiniGauge({ label, value }: { label: string; value: number }) {
  return <div className="rounded-2xl bg-violet-50 px-4 py-3 ring-1 ring-violet-100"><p className="text-xs font-bold uppercase text-violet-500">{label}</p><p className="text-3xl font-black text-violet-950">{value}</p></div>;
}

function StackedBar({ counts }: { counts: ReturnType<typeof testimonyCounts> }) {
  const total = Math.max(counts.Pro + counts.Con + counts.Other, 1);
  return <div><div className="flex h-5 overflow-hidden rounded-full bg-slate-100"><span style={{ width: `${(counts.Pro / total) * 100}%` }} className="bg-emerald-500" /><span style={{ width: `${(counts.Con / total) * 100}%` }} className="bg-rose-500" /><span style={{ width: `${(counts.Other / total) * 100}%` }} className="bg-slate-400" /></div><div className="mt-3 flex flex-wrap gap-3 text-sm text-slate-600"><span>Pro {counts.Pro}</span><span>Con {counts.Con}</span><span>Other {counts.Other}</span></div></div>;
}

function DocketFact({ label, value }: { label: string; value: string }) {
  return <div className="border-l-4 border-blue-700 bg-slate-50 p-4"><dt className="text-xs font-bold uppercase tracking-wider text-slate-500">{label}</dt><dd className="mt-1 font-semibold text-slate-900">{value}</dd></div>;
}

function TestifierTableLite({ testifiers }: { testifiers: Testifier[] }) {
  return <div className="mt-6 overflow-hidden rounded-sm border border-slate-300"><table className="w-full text-left text-sm"><thead className="bg-slate-100 text-xs uppercase tracking-wider text-slate-500"><tr><th className="p-3">Name</th><th className="p-3">Organization</th><th className="p-3">Position</th></tr></thead><tbody className="divide-y divide-slate-200">{testifiers.slice(0, 12).map((t) => <tr key={`${t.raw_name}-${t.raw_organization}`}><td className="p-3 font-medium text-slate-900">{t.raw_name}</td><td className="p-3 text-slate-600">{t.raw_organization || "—"}</td><td className="p-3 text-slate-600">{t.position}</td></tr>)}</tbody></table></div>;
}

function SignalMetric({ label, value, hot = false }: { label: string; value: string; hot?: boolean }) {
  return <div className="rounded-3xl border p-5" style={{ borderColor: hot ? "#F43F5E" : "#1E3A5F", background: hot ? "#3A1020" : "#0B1B2E" }}><p className="font-mono text-xs uppercase tracking-[0.18em] text-slate-400">{label}</p><p className="mt-2 text-4xl font-black text-cyan-50">{value}</p></div>;
}

function SignalCard({ title, value }: { title: string; value: string }) {
  return <div className="rounded-3xl border border-[#1E3A5F] bg-[#0B1B2E] p-5"><p className="font-mono text-xs uppercase tracking-[0.18em] text-cyan-300">{title}</p><p className="mt-2 text-sm font-semibold leading-6 text-slate-200">{value}</p></div>;
}

function ReceiptsPanel({ bundle, style }: { bundle: Bundle; style: StyleConfig }) {
  const isDark = style.bg.startsWith("#0") || style.bg.startsWith("#1");
  return <section className={`rounded-[1.5rem] border p-6 ${isDark ? "bg-white/5" : "bg-white"}`} style={{ borderColor: style.border }}><h2 className={`text-2xl font-black ${isDark ? "text-white" : "text-slate-950"}`}>Receipts</h2><p className={`mt-2 text-sm ${isDark ? "text-slate-300" : "text-slate-600"}`}>Official source families supporting this page.</p><div className="mt-4 flex flex-wrap gap-2">{sourceSystems(bundle).map((s) => <span key={s} className={`rounded-full px-3 py-1.5 font-mono text-xs font-bold ${isDark ? "bg-cyan-300/10 text-cyan-200 ring-1 ring-cyan-300/20" : "bg-slate-100 text-slate-700 ring-1 ring-slate-200"}`}>{s}</span>)}</div></section>;
}

function TranscriptCards({ bundle, accent, dark = false }: { bundle: Bundle; accent: string; dark?: boolean }) {
  const segs = bundle.transcript.segments ?? [];
  return <section className={`rounded-[1.5rem] border p-6 ${dark ? "border-white/10 bg-white/5" : "border-slate-200 bg-white"}`}><h2 className={`text-2xl font-black ${dark ? "text-white" : "text-slate-950"}`}>Timestamped evidence</h2>{segs.length === 0 ? <p className="mt-3 text-sm text-slate-500">No matched transcript segments available.</p> : <div className="mt-5 grid gap-4 md:grid-cols-3">{segs.slice(0, 3).map((s) => <div key={s.start_ms} className={`rounded-2xl p-4 ${dark ? "bg-black/20 text-[#F8EEDF]" : "bg-slate-50 text-slate-800"}`}><TimestampLink bundle={bundle} segment={s} className="font-mono text-xs font-bold underline" color={accent} /><p className="mt-3 text-sm leading-6">{s.text.slice(0, 220)}{s.text.length > 220 ? "…" : ""}</p></div>)}</div>}</section>;
}

function TimestampLink({ bundle, segment, className, color }: { bundle: Bundle; segment: TranscriptSegment; className?: string; color?: string }) {
  const href = bundle.hearing.tvw_event_id ? tvwDeepLink(bundle.hearing.tvw_event_id, segment.start_ms) : bundle.transcript.caption_url || "#";
  return <a href={href} className={className} style={color ? { color } : undefined}>{formatMS(segment.start_ms)} →</a>;
}

function byPosition(testifiers: Testifier[], position: Position): Testifier[] {
  return testifiers.filter((t) => t.position === position);
}

function bestQuote(segments: TranscriptSegment[]): TranscriptSegment | undefined {
  return [...segments].sort((a, b) => b.text.length - a.text.length)[0];
}

function placeholderOrganizations() {
  return [
    {
      canonical_name: "Reviewed organizations will appear here",
      match_confidence: "unmatched" as const,
      testifier_count: 0,
      testifier_position: "Pending",
    },
  ];
}

function signalItems(bundle: Bundle): string[] {
  const counts = testimonyCounts(bundle);
  return [
    `${bundle.bill.bill_id} is currently: ${bundle.status.current || "status unavailable"}.`,
    `${counts.total.toLocaleString()} public sign-ins are attached to this hearing record.`,
    `${counts.Con > counts.Pro ? "Opposition" : counts.Pro > counts.Con ? "Support" : "No single position"} dominates the testimony count.`,
    `${(bundle.transcript.segments ?? []).length.toLocaleString()} timestamped transcript excerpts are available for review.`,
  ];
}
