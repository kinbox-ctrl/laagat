"use client";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { api } from "@/lib/api";
import { useAction } from "@/lib/session";
import { RequireUser } from "@/components/RequireUser";

function Form() {
  const router = useRouter();
  const preset = useSearchParams().get("service") === "estimate" ? "estimate" : "audit";
  const { busy, error, run } = useAction();
  const [f, setF] = useState({ title: "", city: "", state: "", building_type: "Independent house", floors: "G+1", built_up_area_sqft: "", service: preset, quoted_total: "", notes: "" });
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => setF({ ...f, [k]: e.target.value });

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    run(async () => {
      const res = await api("/projects", {
        body: { ...f, built_up_area_sqft: Number(f.built_up_area_sqft) || 0, quoted_total: f.service === "audit" ? Number(f.quoted_total) || 0 : 0 },
      });
      router.push(`/projects/${res.id}`);
    });
  };
  return (
    <div style={{ maxWidth: 640 }} className="stack">
      <div>
        <h1>Tell us about your project</h1>
        <p className="muted">You can upload drawings and quotations on the next screen.</p>
      </div>
      <form className="card" onSubmit={submit}>
        <div className="field">
          <label htmlFor="service">What do you need?</label>
          <select id="service" value={f.service} onChange={set("service")}>
            <option value="audit">Audit my contractor&apos;s estimate or BOQ</option>
            <option value="estimate">Prepare a construction estimate from my drawings</option>
          </select>
        </div>
        <div className="field">
          <label htmlFor="title">Project name</label>
          <input id="title" value={f.title} onChange={set("title")} placeholder="e.g. G+1 house, Sigra" required />
        </div>
        <div className="fields">
          <div><label htmlFor="city">City / town</label><input id="city" value={f.city} onChange={set("city")} required /></div>
          <div><label htmlFor="state">State</label><input id="state" value={f.state} onChange={set("state")} /></div>
        </div>
        <div className="fields" style={{ marginTop: 12 }}>
          <div>
            <label htmlFor="floors">Floors</label>
            <select id="floors" value={f.floors} onChange={set("floors")}><option>G</option><option>G+1</option><option>G+2</option></select>
          </div>
          <div><label htmlFor="area">Built-up area (sq ft)</label><input id="area" type="number" min="0" value={f.built_up_area_sqft} onChange={set("built_up_area_sqft")} /></div>
          {f.service === "audit" && (
            <div><label htmlFor="qt">Contractor&apos;s quoted total (₹)</label><input id="qt" type="number" min="0" value={f.quoted_total} onChange={set("quoted_total")} /></div>
          )}
        </div>
        <div className="field" style={{ marginTop: 12 }}>
          <label htmlFor="notes">Anything we should know? (optional)</label>
          <textarea id="notes" value={f.notes} onChange={set("notes")} placeholder="Specifications, what worries you about the quotation, deadlines…" />
        </div>
        {error && <p className="error" role="alert">{error}</p>}
        <button className="btn" disabled={busy}>{busy ? "Creating…" : "Create project"}</button>
      </form>
    </div>
  );
}

export default function Page() {
  return <RequireUser>{() => <Suspense><Form /></Suspense>}</RequireUser>;
}
