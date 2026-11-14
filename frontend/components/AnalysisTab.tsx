"use client";
import { useCallback, useEffect, useState } from "react";
import { api, date, inr, num } from "@/lib/api";
import { useAction, useSession } from "@/lib/session";
import { Reports } from "./Overview";

const SEV: Record<string, string> = { major: "Major", review: "Review", info: "Note" };

export function AnalysisTab({ project, reload }: { project: any; reload: () => Promise<void> }) {
  const { busy, error, run } = useAction();
  const [data, setData] = useState<any>(null);
  const load = useCallback(() => api(`/projects/${project.id}/analysis`).then(setData), [project.id]);
  useEffect(() => { load().catch(() => {}); }, [load]);

  const analyze = () => run(async () => setData(await api(`/projects/${project.id}/analyze`, { method: "POST" })));
  const approve = () => run(async () => { await api(`/projects/${project.id}/approve`, { method: "POST" }); await load(); await reload(); });
  const deliver = () => run(async () => { await api(`/projects/${project.id}/deliver`, { method: "POST" }); await reload(); });

  const result = data?.run?.result;
  const s = result?.summary;
  const findings: any[] = data?.findings ?? [];
  const open = findings.filter((f) => f.status === "open").length;
  const isAudit = project.service === "audit";

  return (
    <div className="stack">
      <div className="spread">
        <p className="muted small" style={{ maxWidth: 640 }}>
          Deterministic checks on the current BOQ, measurement sheet and reference rates. Re-run after any change; findings whose
          content changes are re-opened for review.
          {data?.run && <> Last run {date(data.run.created_at)}.</>}
        </p>
        <button className="btn" disabled={busy} onClick={analyze}>{busy ? "Working…" : data?.run ? "Re-run analysis" : "Run analysis"}</button>
      </div>
      {error && <p className="error" role="alert">{error}</p>}
      {data && !data.run && <div className="card"><p className="muted">No analysis has been run for this project yet.</p></div>}

      {s && (
        <>
          <div className="card grid3">
            {isAudit && <div className="stat"><div className="label">Contractor quoted (sum of items)</div><div className="value">{inr(s.quoted_total)}</div></div>}
            <div className="stat"><div className="label">{isAudit ? "Independently assessed" : "Estimated cost"}</div><div className="value">{inr(s.assessed_total)}</div></div>
            {isAudit && <div className="stat"><div className="label">Difference to reconcile</div><div className="value">{inr(s.difference)}</div></div>}
            <div className="stat"><div className="label">Value with independent quantities</div><div className="value">{num(s.measured_pct, 1)}%</div></div>
            {isAudit && <div className="stat"><div className="label">Value with a reference rate</div><div className="value">{num(s.rate_checked_pct, 1)}%</div></div>}
          </div>
          {s.unverified_rates && (
            <p className="notice">Some reference prices for {project.city} are unverified placeholders. Rate comparisons that use them are shown as notes and are not applied to the assessed amount. Add verified, dated prices under Rates &amp; rules.</p>
          )}

          <section>
            <h2>Findings <span className="muted small">· {open} open of {findings.length}</span></h2>
            <div className="stack" style={{ gap: 10 }}>
              {findings.length === 0 && <p className="muted">No findings.</p>}
              {findings.map((f) => <Finding key={f.id} f={f} onChange={load} />)}
            </div>
          </section>

          <Assumptions projectId={project.id} />

          <section className="card flush">
            <div className="card-head"><h2>Assessed BOQ</h2></div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr><th>Sl</th><th style={{ minWidth: 220 }}>Description</th><th>Unit</th><th className="num">Quoted qty</th><th className="num">Measured qty</th><th className="num">Quoted rate</th><th className="num">Reference rate</th><th className="num">Quoted amount</th><th className="num">Assessed amount</th><th>Basis</th></tr>
                </thead>
                <tbody>
                  {result.lines.map((l: any) => (
                    <tr key={l.boq_item_id}>
                      <td>{l.sl_no}</td><td>{l.description}</td><td>{l.unit}</td>
                      <td className="num">{num(l.quoted_qty)}</td>
                      <td className="num">{l.measured_qty == null ? "—" : num(l.measured_qty)}</td>
                      <td className="num">{num(l.quoted_rate)}</td>
                      <td className="num">{l.reference_rate == null ? "—" : num(l.reference_rate)}</td>
                      <td className="num">{inr(l.quoted_amount)}</td>
                      <td className="num">{inr(l.assessed_amount)}</td>
                      <td><span className={`badge ${l.qty_basis === "measured" ? "ok" : ""}`}>qty {l.qty_basis}</span> <span className={`badge ${l.rate_basis === "reference" ? "ok" : ""}`}>rate {l.rate_basis}</span></td>
                    </tr>
                  ))}
                  <tr className="total"><td></td><td>Total</td><td colSpan={5}></td><td className="num">{inr(s.quoted_total)}</td><td className="num">{inr(s.assessed_total)}</td><td></td></tr>
                </tbody>
              </table>
            </div>
          </section>

          <div className="grid2">
            <section className="card flush">
              <div className="card-head"><h2>By work category</h2></div>
              <table>
                <thead><tr><th>Category</th><th className="num">Quoted</th><th className="num">Assessed</th></tr></thead>
                <tbody>{s.by_category.map((c: any) => <tr key={c.category}><td>{c.category}</td><td className="num">{inr(c.quoted)}</td><td className="num">{inr(c.assessed)}</td></tr>)}</tbody>
              </table>
            </section>
            <section className="card flush">
              <div className="card-head"><h2>Material and labour requirement</h2></div>
              <table>
                <thead><tr><th>Resource</th><th>Unit</th><th className="num">Quantity</th></tr></thead>
                <tbody>
                  {(s.materials ?? []).map((m: any) => <tr key={m.resource}><td>{m.name}</td><td>{m.unit}</td><td className="num">{num(m.qty)}</td></tr>)}
                  {!s.materials?.length && <tr><td colSpan={3} className="muted">No measurable items with a consumption build-up.</td></tr>}
                </tbody>
              </table>
            </section>
          </div>

          <section className="card">
            <h2>Approve and deliver</h2>
            <p className="small muted">
              Approval re-runs the analysis, requires every finding to be confirmed or dismissed, and freezes a snapshot that the PDF
              and Excel deliverables are generated from. Only confirmed findings appear in the report.
            </p>
            <div className="row">
              <button className="btn" disabled={busy || open > 0} onClick={approve}>Approve and generate report</button>
              <button className="btn secondary" disabled={busy || project.status === "delivered"} onClick={deliver}>
                {project.status === "delivered" ? "Delivered" : "Deliver to customer"}
              </button>
              {open > 0 && <span className="badge review">{open} finding(s) still open</span>}
            </div>
          </section>
          <Reports p={project} staff />
        </>
      )}
    </div>
  );
}

function Finding({ f, onChange }: { f: any; onChange: () => Promise<unknown> }) {
  const { aiEnabled } = useSession();
  const { busy, error, run } = useAction();
  const [note, setNote] = useState(f.analyst_note);
  const [draft, setDraft] = useState(f.ai_draft);
  const setStatus = (status: string) => run(async () => { await api(`/findings/${f.id}`, { method: "PUT", body: { status, analyst_note: note } }); await onChange(); });
  const askAI = () => run(async () => setDraft((await api(`/findings/${f.id}/draft`, { method: "POST" })).ai_draft));

  return (
    <article className={`finding ${f.severity} ${f.status === "dismissed" ? "dismissed" : ""}`}>
      <div className="row">
        <span className={`badge ${f.severity}`}>{SEV[f.severity]}</span>
        <span className="small muted">{f.code}</span>
        {f.amount_impact !== 0 && <span className="small muted">value affected {inr(f.amount_impact)}</span>}
        <span className={`badge ${f.status === "confirmed" ? "ok" : f.status === "open" ? "brand" : ""}`} style={{ marginLeft: "auto" }}>
          {f.status === "open" ? "Needs review" : f.status === "confirmed" ? "Confirmed" : "Dismissed"}
        </span>
      </div>
      <h3>{f.title}</h3>
      <p>{f.explanation}</p>
      <p className="muted">Action: {f.required_action}</p>
      <details>
        <summary>Evidence</summary>
        <pre className="small" style={{ whiteSpace: "pre-wrap", overflowX: "auto" }}>{JSON.stringify(f.evidence, null, 2)}</pre>
      </details>
      <div style={{ marginTop: 10 }}>
        <label htmlFor={`note-${f.id}`}>Reviewer&apos;s note <span className="hint">(printed in the report; required when dismissing)</span></label>
        <textarea id={`note-${f.id}`} value={note} onChange={(e) => setNote(e.target.value)} />
      </div>
      {draft && (
        <div className="notice small" style={{ marginTop: 8 }}>
          <strong>AI draft — check every figure before using.</strong>
          <div style={{ whiteSpace: "pre-wrap", margin: "4px 0" }}>{draft}</div>
          <button className="linkbtn" onClick={() => setNote(draft)}>Copy into reviewer&apos;s note</button>
        </div>
      )}
      {error && <p className="error" role="alert" style={{ marginTop: 8 }}>{error}</p>}
      <div className="row" style={{ marginTop: 10 }}>
        <button className="btn sm" disabled={busy} onClick={() => setStatus("confirmed")}>{f.status === "confirmed" ? "Save note" : "Confirm"}</button>
        <button className="btn secondary sm" disabled={busy} onClick={() => setStatus("dismissed")}>Dismiss</button>
        {f.status !== "open" && <button className="linkbtn" disabled={busy} onClick={() => setStatus("open")}>Re-open</button>}
        {aiEnabled && <button className="linkbtn" disabled={busy} onClick={askAI}>Draft plain-language note with AI</button>}
      </div>
    </article>
  );
}

function Assumptions({ projectId }: { projectId: number }) {
  const { busy, error, run } = useAction();
  const [rows, setRows] = useState<any[]>([]);
  const [kind, setKind] = useState("assumption");
  const [body, setBody] = useState("");
  const load = useCallback(() => api(`/projects/${projectId}/assumptions`).then(setRows), [projectId]);
  useEffect(() => { load().catch(() => {}); }, [load]);
  const add = (e: React.FormEvent) => { e.preventDefault(); run(async () => { await api(`/projects/${projectId}/assumptions`, { body: { kind, body } }); setBody(""); await load(); }); };
  const remove = (id: number) => run(async () => { await api(`/assumptions/${id}`, { method: "DELETE" }); await load(); });
  return (
    <section className="card">
      <h2>Assumptions and exclusions</h2>
      {rows.length === 0 && <p className="muted small">None recorded. State the drawing revisions relied on, assumed specifications and anything outside the assessment.</p>}
      {rows.map((a) => (
        <div key={a.id} className="spread msg">
          <div><span className="badge">{a.kind}</span> {a.body}</div>
          <button className="linkbtn" onClick={() => remove(a.id)}>Delete</button>
        </div>
      ))}
      <form className="row" style={{ marginTop: 10 }} onSubmit={add}>
        <select aria-label="Kind" style={{ width: 140 }} value={kind} onChange={(e) => setKind(e.target.value)}><option value="assumption">Assumption</option><option value="exclusion">Exclusion</option></select>
        <input aria-label="Statement" style={{ flex: 1, minWidth: 220 }} value={body} onChange={(e) => setBody(e.target.value)} required />
        <button className="btn secondary" disabled={busy}>Add</button>
      </form>
      {error && <p className="error" role="alert" style={{ marginTop: 8 }}>{error}</p>}
    </section>
  );
}
