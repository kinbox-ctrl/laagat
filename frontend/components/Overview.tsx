"use client";
import { useCallback, useEffect, useState } from "react";
import { api, date, DOC_KINDS, inr } from "@/lib/api";
import { useAction } from "@/lib/session";

const STEPS = ["submitted", "quoted", "paid", "in_analysis", "in_review", "delivered"];
const STEP_LABEL: Record<string, string> = { submitted: "Submitted", quoted: "Quotation", paid: "Payment", in_analysis: "Analysis", in_review: "Expert review", delivered: "Report delivered" };
const STEP_OF: Record<string, string> = { accepted: "quoted", approved: "in_review" };

export function Overview({ data, staff, reload }: { data: any; staff: boolean; reload: () => Promise<void> }) {
  const p = data.project;
  const at = STEPS.indexOf(STEP_OF[p.status] ?? p.status);
  return (
    <div className="stack">
      <ol className="steps" aria-label="Project progress">
        {STEPS.map((s, i) => (
          <li key={s} className={i < at || p.status === "delivered" ? "done" : i === at ? "current" : ""}>{STEP_LABEL[s]}</li>
        ))}
      </ol>
      <div className="grid2">
        <div className="stack">
          <Order p={p} staff={staff} reload={reload} />
          <Reports p={p} staff={staff} />
        </div>
        <div className="stack">
          <Documents data={data} staff={staff} reload={reload} />
          <Messages projectId={p.id} />
        </div>
      </div>
    </div>
  );
}

function Order({ p, staff, reload }: { p: any; staff: boolean; reload: () => Promise<void> }) {
  const { busy, error, run } = useAction();
  const [q, setQ] = useState({ fee_amount: p.fee_amount || "", delivery_days: p.delivery_days || "", quote_note: p.quote_note || "" });
  const [ref, setRef] = useState("");
  const act = (path: string, body?: unknown) => run(async () => { await api(`/projects/${p.id}/${path}`, { method: "POST", body }); await reload(); });
  const hasQuote = p.fee_amount > 0;

  return (
    <div className="card">
      <h2>Service order</h2>
      {hasQuote ? (
        <table>
          <tbody>
            <tr><td>Service fee</td><td className="num">{inr(p.fee_amount)}</td></tr>
            <tr><td>GST 18%</td><td className="num">{inr(p.gst_amount)}</td></tr>
            <tr className="total"><td>Total payable</td><td className="num">{inr(p.fee_amount + p.gst_amount)}</td></tr>
            <tr><td>Delivery</td><td className="num">{p.delivery_days} working days after payment</td></tr>
            {p.paid_at && <tr><td>Paid</td><td className="num">{date(p.paid_at)} · Invoice {p.invoice_no}</td></tr>}
          </tbody>
        </table>
      ) : (
        <p className="muted">{staff ? "No quotation sent yet." : "We are checking your documents and will send a fee and delivery timeline here."}</p>
      )}
      {p.quote_note && <p className="small muted" style={{ marginTop: 8 }}>{p.quote_note}</p>}

      {!staff && p.status === "quoted" && (
        <p style={{ marginTop: 12 }}><button className="btn" disabled={busy} onClick={() => act("accept")}>Accept quotation</button></p>
      )}
      {!staff && p.status === "accepted" && <p className="notice" style={{ marginTop: 12 }}>Thank you. We will send you a payment link; work starts when payment is confirmed.</p>}

      {staff && !p.paid_at && (
        <form style={{ marginTop: 14 }} onSubmit={(e) => { e.preventDefault(); act("quote", { ...q, fee_amount: Number(q.fee_amount), delivery_days: Number(q.delivery_days) }); }}>
          <div className="fields">
            <div><label htmlFor="fee">Fee (₹, excl. GST)</label><input id="fee" type="number" min="1" value={q.fee_amount} onChange={(e) => setQ({ ...q, fee_amount: e.target.value })} required /></div>
            <div><label htmlFor="days">Delivery (days)</label><input id="days" type="number" min="1" value={q.delivery_days} onChange={(e) => setQ({ ...q, delivery_days: e.target.value })} required /></div>
            <div className="wide"><label htmlFor="qn">Note to customer</label><input id="qn" value={q.quote_note} onChange={(e) => setQ({ ...q, quote_note: e.target.value })} /></div>
          </div>
          <p style={{ marginTop: 10 }}><button className="btn secondary" disabled={busy}>{hasQuote ? "Update quotation" : "Send quotation"}</button></p>
        </form>
      )}
      {staff && hasQuote && !p.paid_at && (
        <form className="row" style={{ marginTop: 6 }} onSubmit={(e) => { e.preventDefault(); act("paid", { payment_ref: ref }); }}>
          <input style={{ maxWidth: 240 }} aria-label="Payment reference" placeholder="Payment gateway reference" value={ref} onChange={(e) => setRef(e.target.value)} required />
          <button className="btn secondary" disabled={busy}>Record payment</button>
        </form>
      )}
      {staff && p.status !== "delivered" && (
        <div className="row" style={{ marginTop: 12 }}>
          <span className="small muted">Stage:</span>
          <button className="btn secondary sm" disabled={busy || p.status === "in_analysis"} onClick={() => act("status", { status: "in_analysis" })}>Start analysis</button>
          <button className="btn secondary sm" disabled={busy || p.status === "in_review"} onClick={() => act("status", { status: "in_review" })}>Send to review</button>
        </div>
      )}
      {error && <p className="error" role="alert" style={{ marginTop: 10 }}>{error}</p>}
    </div>
  );
}

export function Reports({ p, staff }: { p: any; staff: boolean }) {
  const [rows, setRows] = useState<any[]>([]);
  useEffect(() => {
    api(`/projects/${p.id}/reports`).then(setRows).catch(() => {});
  }, [p.id, p.status]);
  return (
    <div className="card">
      <h2>{staff ? "Report versions" : "Your report"}</h2>
      {rows.length === 0 ? (
        <p className="muted">{staff ? "No report has been approved yet." : "Your report will appear here once it has been reviewed and approved."}</p>
      ) : (
        rows.map((r) => (
          <div key={r.id} className="spread msg">
            <div>
              <strong>Version {r.version}</strong>
              <div className="small muted">Approved by {r.approved_by} · {date(r.created_at)}</div>
            </div>
            <div className="row">
              <a className="btn sm" href={`/api/reports/${r.id}/pdf`}>Assessment PDF</a>
              <a className="btn secondary sm" href={`/api/reports/${r.id}/xlsx`}>Excel workbook</a>
            </div>
          </div>
        ))
      )}
    </div>
  );
}

function Documents({ data, staff, reload }: { data: any; staff: boolean; reload: () => Promise<void> }) {
  const p = data.project;
  const { busy, error, run } = useAction();
  const [kind, setKind] = useState(p.service === "audit" ? "quotation" : "drawing_architectural");
  const [file, setFile] = useState<File | null>(null);
  const [inputKey, setInputKey] = useState(0);
  const processing = data.documents.some((d: any) => d.status === "queued");
  useEffect(() => {
    if (!processing) return;
    const t = setInterval(reload, 1500);
    return () => clearInterval(t);
  }, [processing, reload]);

  const upload = (e: React.FormEvent) => {
    e.preventDefault();
    if (!file) return;
    run(async () => {
      const form = new FormData();
      form.append("kind", kind);
      form.append("file", file);
      await api(`/projects/${p.id}/documents`, { form });
      setFile(null);
      setInputKey((k) => k + 1);
      await reload();
    });
  };
  const extract = (id: number) => run(async () => { await api(`/projects/${p.id}/boq/import`, { body: { document_id: id } }); await reload(); });

  return (
    <div className="card">
      <h2>Documents</h2>
      {data.documents.length === 0 && <p className="muted">Nothing uploaded yet. Add your drawings{p.service === "audit" ? ", contractor quotation and BOQ" : " and specifications"}.</p>}
      {data.documents.map((d: any) => (
        <div key={d.id} className="msg">
          <div className="spread">
            <div>
              <a href={`/api/documents/${d.id}/download`}><strong>{d.filename}</strong></a>
              <div className="small muted">{DOC_KINDS[d.kind]} · rev {d.revision} · {(d.size / 1024).toFixed(0)} KB · {date(d.created_at)}</div>
            </div>
            {staff && /\.(xlsx|csv|pdf)$/i.test(d.filename) && (
              <button className="btn secondary sm" disabled={busy || d.status === "queued"} onClick={() => extract(d.id)}>
                {d.status === "queued" ? "Extracting…" : d.status === "processed" ? "Re-extract BOQ" : "Extract BOQ rows"}
              </button>
            )}
          </div>
          {staff && d.note && <div className={`small ${d.status === "failed" ? "error" : "muted"}`} style={{ marginTop: 6 }}>{d.note}</div>}
        </div>
      ))}
      <form onSubmit={upload} style={{ marginTop: 12 }}>
        <div className="fields">
          <div>
            <label htmlFor="kind">Document type</label>
            <select id="kind" value={kind} onChange={(e) => setKind(e.target.value)}>
              {Object.entries(DOC_KINDS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </select>
          </div>
          <div className="wide">
            <label htmlFor="file">File</label>
            <input key={inputKey} id="file" type="file" accept=".pdf,.dwg,.dxf,.jpg,.jpeg,.png,.xlsx,.csv,.docx" onChange={(e) => setFile(e.target.files?.[0] ?? null)} required />
          </div>
        </div>
        <div className="hint">PDF, DWG, DXF, JPG, PNG, XLSX, CSV or DOCX, up to 50 MB.</div>
        {error && <p className="error" role="alert" style={{ marginTop: 8 }}>{error}</p>}
        <p style={{ marginTop: 10 }}><button className="btn secondary" disabled={busy || !file}>{busy ? "Uploading…" : "Upload"}</button></p>
      </form>
    </div>
  );
}

function Messages({ projectId }: { projectId: number }) {
  const [rows, setRows] = useState<any[]>([]);
  const [body, setBody] = useState("");
  const { busy, error, run } = useAction();
  const load = useCallback(() => api(`/projects/${projectId}/messages`).then(setRows).catch(() => {}), [projectId]);
  useEffect(() => { load(); }, [load]);
  const send = (e: React.FormEvent) => {
    e.preventDefault();
    run(async () => { await api(`/projects/${projectId}/messages`, { body: { body } }); setBody(""); await load(); });
  };
  return (
    <div className="card">
      <h2>Questions and clarifications</h2>
      {rows.length === 0 && <p className="muted">No messages yet.</p>}
      {rows.map((m) => (
        <div key={m.id} className="msg">
          <div className="small muted">{m.author}{m.role !== "customer" ? " (BuildCost)" : ""} · {date(m.created_at)}</div>
          <div style={{ whiteSpace: "pre-wrap" }}>{m.body}</div>
        </div>
      ))}
      <form onSubmit={send} style={{ marginTop: 10 }}>
        <textarea aria-label="Your message" value={body} onChange={(e) => setBody(e.target.value)} placeholder="Ask a question about your project or report" required />
        {error && <p className="error" role="alert">{error}</p>}
        <p style={{ marginTop: 8 }}><button className="btn secondary" disabled={busy}>Send</button></p>
      </form>
    </div>
  );
}
