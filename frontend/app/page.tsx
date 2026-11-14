import Link from "next/link";

export default function Home() {
  return (
    <div className="stack">
      <section className="hero">
        <div className="eyebrow">Independent construction cost assessment</div>
        <h1>Know what your house should cost before you sign with a contractor.</h1>
        <p>
          We check your contractor&apos;s quotation against your drawings, measured quantities and dated material prices,
          and tell you which figures to question. We don&apos;t build, sell materials or take commissions from contractors.
        </p>
      </section>

      <section className="grid2">
        <div className="card">
          <div className="eyebrow">Service 1</div>
          <h2>Audit my contractor&apos;s estimate</h2>
          <p className="muted">You have a quotation or BOQ and want it checked before committing.</p>
          <ul className="plain small">
            <li>Arithmetic, duplicate and overlapping items</li>
            <li>Quantities compared with an independent takeoff from your drawings</li>
            <li>Rates compared with documented reference build-ups</li>
            <li>Work that is missing or hidden inside lump sums</li>
            <li>A list of questions to settle with the contractor</li>
          </ul>
          <p style={{ marginTop: 14 }}>
            <Link href="/projects/new?service=audit" className="btn">Audit my estimate</Link>
          </p>
        </div>
        <div className="card">
          <div className="eyebrow">Service 2</div>
          <h2>Prepare my construction estimate</h2>
          <p className="muted">You have drawings but no proper BOQ yet.</p>
          <ul className="plain small">
            <li>Quantity takeoff with measurement sheets</li>
            <li>Detailed BOQ in Excel</li>
            <li>Material and labour requirement</li>
            <li>Rate analysis and abstract of cost</li>
            <li>Reviewed by an estimator before delivery</li>
          </ul>
          <p style={{ marginTop: 14 }}>
            <Link href="/projects/new?service=estimate" className="btn">Prepare my estimate</Link>
          </p>
        </div>
      </section>

      <section className="card">
        <h2>How it works</h2>
        <div className="grid3 small">
          <div><strong>1. Submit your project.</strong> Location, size, and your drawings, quotation or BOQ.</div>
          <div><strong>2. Get a fee and timeline.</strong> We check what you sent and quote a fixed fee.</div>
          <div><strong>3. We measure and check.</strong> Calculations are deterministic and reviewed by an estimator.</div>
          <div><strong>4. Receive your report.</strong> A PDF assessment and an Excel workbook, then ask us questions.</div>
        </div>
      </section>

      <p className="muted small">
        Currently for independent houses up to G+2. Reports are desk assessments of the documents you provide: they are
        not structural design checks or site inspections, and an identified difference is not a guaranteed saving.
      </p>
    </div>
  );
}
