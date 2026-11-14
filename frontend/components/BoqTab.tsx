"use client";
import { useCallback, useEffect, useState } from "react";
import { api, inr, num } from "@/lib/api";
import { useAction, useSession } from "@/lib/session";

type Draft = { sl_no: string; description: string; unit: string; quantity: string; rate: string; amount: string; work_item: string };
const blank: Draft = { sl_no: "", description: "", unit: "", quantity: "", rate: "", amount: "", work_item: "auto" };

const toBody = (d: Draft) => ({
  sl_no: d.sl_no, description: d.description, unit: d.unit,
  quantity: Number(d.quantity) || 0, rate: Number(d.rate) || 0, amount: Number(d.amount) || 0,
  ...(d.work_item === "auto" ? {} : { work_item: d.work_item }),
});

export function BoqTab({ project, catalog, reload }: { project: any; catalog: any; reload: () => Promise<void> }) {
  const { aiEnabled } = useSession();
  const { busy, error, run } = useAction();
  const [rows, setRows] = useState<any[]>([]);
  const [editing, setEditing] = useState<number | null>(null);
  const [draft, setDraft] = useState<Draft>(blank);
  const [add, setAdd] = useState<Draft>(blank);
  const [total, setTotal] = useState(String(project.quoted_total || ""));
  const [notice, setNotice] = useState("");

  const load = useCallback(() => api(`/projects/${project.id}/boq`).then(setRows), [project.id]);
  useEffect(() => { load().catch(() => {}); }, [load]);

  const name = (code: string) => catalog.items.find((w: any) => w.code === code)?.name ?? code;
  const sum = rows.reduce((s, r) => s + r.amount, 0);
  const unmapped = rows.filter((r) => !r.work_item).length;
  const isAudit = project.service === "audit";

  const startEdit = (r: any) => {
    setEditing(r.id);
    setDraft({ sl_no: r.sl_no, description: r.description, unit: r.unit, quantity: String(r.quantity), rate: String(r.rate), amount: String(r.amount), work_item: r.work_item });
  };
  const save = () => run(async () => { await api(`/boq/${editing}`, { method: "PUT", body: toBody(draft) }); setEditing(null); await load(); });
  const remove = (id: number) => run(async () => { await api(`/boq/${id}`, { method: "DELETE" }); await load(); });
  const accept = (r: any) => run(async () => {
    await api(`/boq/${r.id}`, { method: "PUT", body: { sl_no: r.sl_no, description: r.description, unit: r.unit, quantity: r.quantity, rate: r.rate, amount: r.amount, work_item: r.ai_suggestion } });
    await load();
  });
  const create = (e: React.FormEvent) => { e.preventDefault(); run(async () => { await api(`/projects/${project.id}/boq`, { body: toBody(add) }); setAdd(blank); await load(); }); };
  const suggest = () => run(async () => { const r = await api(`/projects/${project.id}/boq/suggest`, { method: "POST" }); setNotice(`AI suggested a work item for ${r.suggested} line(s). Review and accept each one.`); await load(); });
  const fromTakeoff = () => run(async () => { const r = await api(`/projects/${project.id}/boq/from-takeoff`, { method: "POST" }); setNotice(`${r.items} BOQ line(s) built from the measurement sheet.`); await load(); });
  const saveTotal = () => run(async () => { await api(`/projects/${project.id}/quoted-total`, { method: "PUT", body: { quoted_total: Number(total) || 0 } }); await reload(); });

  const itemSelect = (value: string, onChange: (v: string) => void, withAuto: boolean) => (
    <select aria-label="Work item" value={value} onChange={(e) => onChange(e.target.value)}>
      {withAuto && <option value="auto">Classify automatically</option>}
      <option value="">Not mapped</option>
      {catalog.items.map((w: any) => <option key={w.code} value={w.code}>{w.code} · {w.name}</option>)}
    </select>
  );
  const cell = (d: Draft, set: (d: Draft) => void, k: keyof Draft, label: string, type = "text") => (
    <input aria-label={label} type={type} step="any" min={type === "number" ? 0 : undefined} value={d[k]} onChange={(e) => set({ ...d, [k]: e.target.value })} />
  );

  return (
    <div className="stack">
      <div className="spread">
        <p className="muted small" style={{ maxWidth: 640 }}>
          {isAudit
            ? "The contractor's BOQ as quoted. Extracted rows are drafts: correct them against the source document, then map each line to a supported work item so it can be checked."
            : "The estimate BOQ. Build it from the measurement sheet, then set rates for items without a reference build-up."}
        </p>
        <div className="row">
          {!isAudit && <button className="btn secondary" disabled={busy} onClick={fromTakeoff}>Build from takeoff</button>}
          {aiEnabled && unmapped > 0 && <button className="btn secondary" disabled={busy} onClick={suggest}>Suggest mappings with AI ({unmapped})</button>}
        </div>
      </div>
      {notice && <p className="notice">{notice}</p>}
      {error && <p className="error" role="alert">{error}</p>}

      <div className="card flush table-wrap">
        <table>
          <thead>
            <tr><th>Sl</th><th style={{ minWidth: 260 }}>Description</th><th>Unit</th><th className="num">Qty</th><th className="num">Rate</th><th className="num">Amount</th><th>Work item</th><th></th></tr>
          </thead>
          <tbody>
            {rows.length === 0 && <tr><td colSpan={8} className="muted">No BOQ lines yet. Extract them from an uploaded document on the Overview tab, or add them below.</td></tr>}
            {rows.map((r) => editing === r.id ? (
              <tr key={r.id}>
                <td>{cell(draft, setDraft, "sl_no", "Serial number")}</td>
                <td>{cell(draft, setDraft, "description", "Description")}</td>
                <td>{cell(draft, setDraft, "unit", "Unit")}</td>
                <td>{cell(draft, setDraft, "quantity", "Quantity", "number")}</td>
                <td>{cell(draft, setDraft, "rate", "Rate", "number")}</td>
                <td>{cell(draft, setDraft, "amount", "Amount", "number")}</td>
                <td>{itemSelect(draft.work_item, (v) => setDraft({ ...draft, work_item: v }), false)}</td>
                <td style={{ whiteSpace: "nowrap" }}><button className="btn sm" style={{ marginRight: 8 }} disabled={busy} onClick={save}>Save</button><button className="linkbtn" onClick={() => setEditing(null)}>Cancel</button></td>
              </tr>
            ) : (
              <tr key={r.id}>
                <td>{r.sl_no}</td>
                <td>{r.description}</td>
                <td>{r.unit}</td>
                <td className="num">{num(r.quantity)}</td>
                <td className="num">{num(r.rate)}</td>
                <td className="num">{inr(r.amount)}</td>
                <td>
                  {r.work_item ? <span title={`Mapped by ${r.mapping_source}`}>{name(r.work_item)}</span> : <span className="badge review">Not mapped</span>}
                  {!r.work_item && r.ai_suggestion && (
                    <div className="small" style={{ marginTop: 4 }}>AI suggests {name(r.ai_suggestion)} · <button className="linkbtn" disabled={busy} onClick={() => accept(r)}>Accept</button></div>
                  )}
                </td>
                <td style={{ whiteSpace: "nowrap" }}><button className="linkbtn" style={{ marginRight: 10 }} onClick={() => startEdit(r)}>Edit</button><button className="linkbtn" onClick={() => remove(r.id)}>Delete</button></td>
              </tr>
            ))}
            {rows.length > 0 && <tr className="total"><td></td><td>Sum of items</td><td></td><td></td><td></td><td className="num">{inr(sum)}</td><td></td><td></td></tr>}
          </tbody>
        </table>
      </div>

      {isAudit && (
        <div className="card">
          <h2>Total stated on the quotation</h2>
          <div className="row">
            <input style={{ maxWidth: 200 }} type="number" min="0" aria-label="Quoted total" value={total} onChange={(e) => setTotal(e.target.value)} />
            <button className="btn secondary" disabled={busy} onClick={saveTotal}>Save</button>
            {project.quoted_total > 0 && Math.abs(project.quoted_total - sum) > 1 && rows.length > 0 && (
              <span className="badge review">Differs from sum of items by {inr(project.quoted_total - sum)}</span>
            )}
          </div>
        </div>
      )}

      <form className="card" onSubmit={create}>
        <h2>Add a BOQ line</h2>
        <div className="fields">
          <div><label>Sl</label>{cell(add, setAdd, "sl_no", "Serial number")}</div>
          <div className="wide"><label>Description</label>{cell(add, setAdd, "description", "Description")}</div>
          <div><label>Unit</label>{cell(add, setAdd, "unit", "Unit")}</div>
          <div><label>Quantity</label>{cell(add, setAdd, "quantity", "Quantity", "number")}</div>
          <div><label>Rate</label>{cell(add, setAdd, "rate", "Rate", "number")}</div>
          <div><label>Amount</label>{cell(add, setAdd, "amount", "Amount", "number")}</div>
          <div className="wide"><label>Work item</label>{itemSelect(add.work_item, (v) => setAdd({ ...add, work_item: v }), true)}</div>
        </div>
        <div className="hint">Leave amount blank to use quantity × rate. Units such as cum, cft, sqm, sqft, rmt, kg, MT, nos and LS are recognised.</div>
        <p style={{ marginTop: 10 }}><button className="btn secondary" disabled={busy}>Add line</button></p>
      </form>
    </div>
  );
}
