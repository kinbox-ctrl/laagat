"use client";
import { useCallback, useEffect, useState } from "react";
import { api, num } from "@/lib/api";
import { useAction } from "@/lib/session";
import { RequireUser } from "@/components/RequireUser";

const today = () => new Date().toISOString().slice(0, 10);

function Rates() {
  const { busy, error, run } = useAction();
  const [rates, setRates] = useState<any[]>([]);
  const [catalog, setCatalog] = useState<any>(null);
  const [f, setF] = useState({ resource: "", location: "", rate: "", effective_date: today(), source: "", spec: "", incl_tax: true, verified: false });
  const load = useCallback(() => api("/rates").then(setRates), []);
  useEffect(() => { load().catch(() => {}); api("/catalog").then(setCatalog).catch(() => {}); }, [load]);
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setF({ ...f, [k]: e.target.value });
  const res = (code: string) => catalog?.resources.find((r: any) => r.code === code);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    run(async () => { await api("/rates", { body: { ...f, rate: Number(f.rate) } }); setF({ ...f, rate: "", source: f.source }); await load(); });
  };
  const remove = (id: number) => run(async () => { await api(`/rates/${id}`, { method: "DELETE" }); await load(); });

  return (
    <div className="stack">
      <div>
        <h1>Rates &amp; rules</h1>
        <p className="muted" style={{ maxWidth: 720 }}>
          Reference prices are tied to a location, a date and a source. A project uses the most recent price for its city, then its
          state, then DEFAULT. Only prices marked verified are applied to an assessed estimate.
        </p>
      </div>
      {error && <p className="error" role="alert">{error}</p>}

      <form className="card" onSubmit={submit}>
        <h2>Add a reference price</h2>
        <div className="fields">
          <div className="wide">
            <label htmlFor="res">Resource</label>
            <select id="res" value={f.resource} onChange={set("resource")} required>
              <option value="">Choose…</option>
              {catalog?.resources.map((r: any) => <option key={r.code} value={r.code}>{r.name} (per {r.unit})</option>)}
            </select>
          </div>
          <div><label htmlFor="loc">Location</label><input id="loc" value={f.location} onChange={set("location")} placeholder="City, state or DEFAULT" required /></div>
          <div><label htmlFor="rate">Rate (₹)</label><input id="rate" type="number" step="any" min="0" value={f.rate} onChange={set("rate")} required /></div>
          <div><label htmlFor="dt">Effective date</label><input id="dt" type="date" value={f.effective_date} onChange={set("effective_date")} required /></div>
          <div className="wide"><label htmlFor="srcr">Source</label><input id="srcr" value={f.source} onChange={set("source")} placeholder="Supplier, quotation reference or published schedule" required /></div>
          <div><label htmlFor="spec">Specification</label><input id="spec" value={f.spec} onChange={set("spec")} placeholder="Brand, grade, delivery" /></div>
        </div>
        <div className="row" style={{ marginTop: 8 }}>
          <label style={{ fontWeight: 400, margin: 0 }}><input type="checkbox" checked={f.incl_tax} onChange={(e) => setF({ ...f, incl_tax: e.target.checked })} /> Includes GST</label>
          <label style={{ fontWeight: 400, margin: 0 }}><input type="checkbox" checked={f.verified} onChange={(e) => setF({ ...f, verified: e.target.checked })} /> Verified against the source document</label>
          <button className="btn secondary" disabled={busy}>Add price</button>
        </div>
      </form>

      <div className="card flush table-wrap">
        <table>
          <thead><tr><th>Resource</th><th>Location</th><th className="num">Rate</th><th>Date</th><th style={{ minWidth: 240 }}>Source</th><th>Status</th><th></th></tr></thead>
          <tbody>
            {rates.map((r) => (
              <tr key={r.id}>
                <td>{res(r.resource)?.name ?? r.resource}<div className="small muted">per {res(r.resource)?.unit}{r.spec ? ` · ${r.spec}` : ""}</div></td>
                <td>{r.location}</td><td className="num">{num(r.rate)}</td><td>{r.effective_date}</td>
                <td className="small">{r.source}</td>
                <td><span className={`badge ${r.verified ? "ok" : "review"}`}>{r.verified ? "Verified" : "Unverified"}</span></td>
                <td><button className="linkbtn" onClick={() => remove(r.id)}>Delete</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {catalog && (
        <section className="card flush">
          <div className="card-head">
            <h2>Supported work items · rule set {catalog.rule_version}</h2>
            <p className="small muted" style={{ margin: "4px 0 0" }}>
              Measurement rules and consumption per unit of work. Rate = (materials + labour) + {catalog.overhead_profit_pct}% overhead and profit.
              These constants are draft textbook values pending sign-off by a quantity surveyor; they are versioned in code (backend/internal/calc/catalog.go).
            </p>
          </div>
          <div className="table-wrap">
            <table>
              <thead><tr><th>Work item</th><th>Unit</th><th style={{ minWidth: 240 }}>Measurement rule</th><th style={{ minWidth: 280 }}>Consumption per unit</th></tr></thead>
              <tbody>
                {catalog.items.map((w: any) => (
                  <tr key={w.code}>
                    <td><strong>{w.name}</strong><div className="small muted">{w.code} · {w.category}{w.required ? " · expected in every house quotation" : ""}</div></td>
                    <td>{w.unit}</td>
                    <td className="small">{w.rule}</td>
                    <td className="small">
                      {w.coefficients?.length
                        ? w.coefficients.map((c: any) => <div key={c.resource} title={c.basis}>{num(c.qty, 4)} {res(c.resource)?.unit} {res(c.resource)?.name}</div>)
                        : <span className="muted">No rate build-up: compared against quotations only</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  );
}

export default function Page() {
  return <RequireUser staff>{() => <Rates />}</RequireUser>;
}
