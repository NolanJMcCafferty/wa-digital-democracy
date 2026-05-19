import Link from "next/link";

export default function MethodologyPage() {
  return (
    <article className="prose prose-stone max-w-3xl space-y-8">
      <header className="space-y-3">
        <p className="text-sm font-medium uppercase tracking-wider text-stone-500">
          Methodology
        </p>
        <h1 className="text-4xl font-bold tracking-tight text-stone-900">
          How WA Digital Democracy works
        </h1>
        <p className="text-lg text-stone-700">
          WA Digital Democracy helps people follow Washington public decisions
          by connecting bills, hearings, testimony, video, transcripts,
          organizations, and public money records. The goal is simple: make it
          easier to see what happened, who participated, and where the original
          evidence lives.
        </p>
      </header>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          We start with official public records
        </h2>
        <p className="text-stone-700">
          The site is built from public government sources: legislative bill
          records, committee hearing schedules, public testimony sign-ins,
          TVW hearing video and captions, campaign-finance and lobbying data,
          contracts, budgets, and other public datasets where available.
        </p>
        <p className="text-stone-700">
          Wherever possible, pages link back to the original record — an
          official bill page, hearing source, TVW video, transcript/caption
          file, or public dataset row. You should be able to check important
          claims against the source rather than taking our word for it.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          We connect records that usually live apart
        </h2>
        <p className="text-stone-700">
          Washington publishes a lot of civic data, but it is fragmented. A
          bill, a hearing video, a testimony sign-in sheet, a lobbying record,
          and a contract record may all live in different systems. WA Digital
          Democracy brings those pieces together so a user can move from a bill
          to the people and organizations involved, the testimony around it,
          and related public-record context.
        </p>
        <p className="text-stone-700">
          These connections are not always obvious. Names may be abbreviated,
          misspelled, or entered differently across systems. When a match is
          uncertain, we label it cautiously or hold it for review instead of
          presenting it as fact.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          We use transcripts carefully
        </h2>
        <p className="text-stone-700">
          Hearing transcripts come from TVW caption files and, where useful,
          speech-processing tools that help separate who spoke when. Captions
          and automated transcripts can contain mistakes, so transcript text is
          treated as evidence to inspect — not as a perfect official quote.
        </p>
        <p className="text-stone-700">
          Speaker names are especially sensitive. Automated tools may identify
          anonymous speaker clusters, but they do not know who those speakers
          are. Named speaker labels should come from reviewable evidence, such
          as a person introducing themselves, official rosters, public testimony
          records, or human review.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          We show confidence and limits
        </h2>
        <ul className="list-disc space-y-2 pl-6 text-stone-700">
          <li>
            <strong>Confirmed facts</strong> come directly from official public
            records or reviewed matches.
          </li>
          <li>
            <strong>Likely matches</strong> are useful leads, but they should be
            treated with caution until reviewed.
          </li>
          <li>
            <strong>Unknown speakers or organizations</strong> stay unknown
            rather than being guessed into certainty.
          </li>
          <li>
            <strong>Money, lobbying, and contract records</strong> are shown as
            context. They do not prove why someone testified or why a policy
            moved.
          </li>
        </ul>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          What each page is trying to answer
        </h2>
        <ul className="list-disc space-y-2 pl-6 text-stone-700">
          <li>
            <strong>Bill pages</strong> explain what the proposal does, where it
            is in the process, who sponsored it, and what hearings/testimony are
            connected to it.
          </li>
          <li>
            <strong>Hearing pages</strong> bring together the agenda, testimony
            positions, video, and transcript excerpts for a public meeting.
          </li>
          <li>
            <strong>Organization pages</strong> show where an organization
            appears in testimony and, when confidently matched, related public
            lobbying, campaign-finance, contract, or spending context.
          </li>
          <li>
            <strong>Issue pages</strong> collect related bills, hearings,
            testimony, organizations, and source coverage around a topic.
          </li>
        </ul>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl font-semibold text-stone-900">
          What this site is not
        </h2>
        <ul className="list-disc space-y-2 pl-6 text-stone-700">
          <li>
            <strong>Not the official record.</strong> The official source remains
            the legislature, TVW, the Public Disclosure Commission, and other
            public agencies. This site points you back to them.
          </li>
          <li>
            <strong>Not a claim of causation.</strong> Showing testimony,
            lobbying, donations, contracts, or spending side by side does not
            prove one caused another.
          </li>
          <li>
            <strong>Not complete.</strong> Coverage depends on what public
            sources expose and what has been processed so far.
          </li>
          <li>
            <strong>Not a replacement for human judgment.</strong> Automated
            matching and transcription help organize records, but important
            names, quotes, and entity links should be checked against sources.
          </li>
        </ul>
      </section>

      <section className="space-y-4 rounded-lg border border-stone-300 bg-white p-5">
        <h2 className="text-xl font-semibold text-stone-900">
          Want to inspect the sources?
        </h2>
        <p className="text-stone-700">
          Visit the{" "}
          <Link href="/sources" className="text-blue-700 underline hover:text-blue-900">
            Sources page
          </Link>{" "}
          for the public source families currently used by the site and the
          limitations of each.
        </p>
      </section>
    </article>
  );
}
