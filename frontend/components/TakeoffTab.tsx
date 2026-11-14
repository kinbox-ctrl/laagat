"use client";
import { Fragment, useCallback, useEffect, useState } from "react";
import { api, num } from "@/lib/api";
import { useAction } from "@/lib/session";

const METHODS: Record<string, { label: string; l: string; b?: string; d?: string; dia?: boolean; hint: string }> = {
  lbd: { label: "Nos × L × B × D", l: "Length (m)", b: "Breadth (m)", d: "Depth / height (m)", hint: "Leave a dimension blank to measure an area or a length." },
  steel: { label: "Bar bending (steel)", l: "Cutting length (m)", dia: true, hint: "Weight = nos × length × d²/162 kg." },
  opening: { label: "Opening (IS 1200 deduction)", l: "Opening width (m)", b: "Wall thickness (m)", d: "Opening height (m)", hint: "The IS 1200 opening rule for the work item is applied automatically." },
  direct: { label: "Counted / direct quantity", l: "Quantity each", hint: "For enumerated items such as doors." },
};
const blank = { work_item: "", method: "lbd", description: "", nos: "1", length: "", breadth: "", depth: "", dia_mm: "", is_deduction: false, remarks: "", document_id: "" };

export function TakeoffTab({ project, catalog, documents }: { project: any; catalog: any; documents: any[] }) {
  const { busy, error, run } = useAction();
  const [rows, setRows] = useState<any[]>([]);
  const [f, setF] = useState(blank);
  const load = useCallback(() => api(`/projects/${project.id}/measurements`).then(setRows), [project.id]);
  useEffect(() => { load().catch(() => {}); }, [load]);

  const items = catalog.items.filter((w: any) => w.unit !== "ls");
  const item = (code: string) => catalog.items.find((w: any) => w.code === code);
  const m = METHODS[f.method];
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setF({ ...f, [k]: e.target.value });

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    run(async () => {
      await api(`/projects/${project.id}/measurements`, {
        body: {
          work_item: f.work_item, method: f.method, description: f.description, remarks: f.remarks, is_deduction: f.method === "lbd" && f.is_deduction,
          nos: Number(f.nos) || 1, length: Number(f.length) || 0, breadth: Number(f.breadth) || 0, depth: Number(f.depth) || 0, dia_mm: Number(f.dia_mm) || 0,
          document_id: f.document_id ? Number(f.document_id) : null,
        },
      });
      setF({ ...f, description: "", length: "", breadth: "", depth: "", remarks: "", is_deduction: false });
      await load();
    });
  };
  const remove = (id: number) => run(async () => { await api(`/measurements/${id}`, { method: "DELETE" }); await load(); });

  const groups: Record<string, any[]> = {};
  rows.forEach((r) => (groups[r.work_item] ??= []).push(r));
  const drawings = documents.filter((d) => d.kind.startsWith("drawing"));

  return (
    <div className="stack">
      <p className="muted small" style={{ maxWidth: 720 }}>
        Measurement sheet: quantities measured from the drawings, one line per dimension set. Every quantity is computed by the
        calculation engine and keeps its formula. A takeoff for a work item must cover the whole item: a partial takeoff
        understates it and will show as a quantity variance.
      </p>
      {error && <p className="error" role="alert">{error}</p>}

      <form className="card" onSubmit={submit}>
        <h2>Add a measurement line</h2>
        <div className="fields">
          <div className="wide">
            <label htmlFor="wi">Work item</label>
            <select id="wi" value={f.work_item} onChange={set("work_item")} required>
              <option value="">Choose…</option>
              {items.map((w: any) => <option key={w.code} value={w.code}>{w.name} ({w.unit})</option>)}
            </select>
          </div>
          <div>
            <label htmlFor="method">Method</label>
            <select id="method" value={f.method} onChange={set("method")}>
              {Object.entries(METHODS).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
            </select>
          </div>
          <div className="wide"><label htmlFor="desc">Description / location</label><input id="desc" value={f.description} onChange={set("description")} placeholder="e.g. Footing F1, grid A-1 to A-4" required /></div>
          <div><label htmlFor="nos">Nos</label><input id="nos" type="number" step="any" min="0" value={f.nos} onChange={set("nos")} /></div>
          <div><label htmlFor="len">{m.l}</label><input id="len" type="number" step="any" min="0" value={f.length} onChange={set("length")} /></div>
          {m.b && <div><label htmlFor="br">{m.b}</label><input id="br" type="number" step="any" min="0" value={f.breadth} onChange={set("breadth")} /></div>}
          {m.d && <div><label htmlFor="dp">{m.d}</label><input id="dp" type="number" step="any" min="0" value={f.depth} onChange={set("depth")} /></div>}
          {m.dia && <div><label htmlFor="dia">Bar diameter (mm)</label><input id="dia" type="number" step="any" min="0" value={f.dia_mm} onChange={set("dia_mm")} /></div>}
          {drawings.length > 0 && (
            <div>
              <label htmlFor="src">Source drawing</label>
              <select id="src" value={f.document_id} onChange={set("document_id")}>
                <option value="">Not specified</option>
                {drawings.map((d) => <option key={d.id} value={d.id}>{d.filename} (rev {d.revision})</option>)}
              </select>
            </div>
          )}
          <div><label htmlFor="rem">Remarks</label><input id="rem" value={f.remarks} onChange={set("remarks")} /></div>
        </div>
        <div className="hint">{m.hint}{f.work_item && item(f.work_item) ? ` Rule: ${item(f.work_item).rule}.` : ""}</div>
        {f.method === "lbd" && (
          <label style={{ fontWeight: 400, marginTop: 8 }}><input type="checkbox" checked={f.is_deduction} onChange={(e) => setF({ ...f, is_deduction: e.target.checked })} /> This line is a deduction</label>
        )}
        <p style={{ marginTop: 10 }}><button className="btn secondary" disabled={busy}>Add line</button></p>
      </form>

      <div className="card flush table-wrap">
        <table>
          <thead><tr><th style={{ minWidth: 220 }}>Description</th><th className="num">Nos</th><th className="num">L</th><th className="num">B</th><th className="num">D</th><th>Calculation</th><th className="num">Quantity</th><th></th></tr></thead>
          <tbody>
            {rows.length === 0 && <tr><td colSpan={8} className="muted">No measurements yet.</td></tr>}
            {Object.entries(groups).map(([code, lines]) => (
              <Fragment key={code}>
                <tr className="sub">
                  <td colSpan={6}>{item(code)?.name ?? code}</td>
                  <td className="num">{num(lines.reduce((s, l) => s + l.quantity, 0), 3)} {item(code)?.unit}</td>
                  <td></td>
                </tr>
                {lines.map((l) => (
                  <tr key={l.id}>
                    <td>{l.description}{l.remarks && <div className="small muted">{l.remarks}</div>}</td>
                    <td className="num">{num(l.nos)}</td>
                    <td className="num">{l.length ? num(l.length, 3) : ""}</td>
                    <td className="num">{l.breadth ? num(l.breadth, 3) : ""}</td>
                    <td className="num">{l.depth ? num(l.depth, 3) : ""}</td>
                    <td className="small">{l.formula}</td>
                    <td className="num">{num(l.quantity, 3)}</td>
                    <td><button className="linkbtn" onClick={() => remove(l.id)}>Delete</button></td>
                  </tr>
                ))}
              </Fragment>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
