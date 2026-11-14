"use client";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { api, inr, isStaff, STATUS_LABEL, User } from "@/lib/api";
import { RequireUser } from "@/components/RequireUser";
import { Overview } from "@/components/Overview";
import { BoqTab } from "@/components/BoqTab";
import { TakeoffTab } from "@/components/TakeoffTab";
import { AnalysisTab } from "@/components/AnalysisTab";

const TABS = [["overview", "Overview"], ["boq", "BOQ"], ["takeoff", "Quantity takeoff"], ["analysis", "Analysis & report"]];

function Project({ user }: { user: User }) {
  const { id } = useParams<{ id: string }>();
  const staff = isStaff(user);
  const [data, setData] = useState<any>(null);
  const [catalog, setCatalog] = useState<any>(null);
  const [error, setError] = useState("");
  const [tab, setTab] = useState("overview");

  const reload = useCallback(async () => {
    try {
      setData(await api(`/projects/${id}`));
    } catch (e: any) {
      setError(e.message);
    }
  }, [id]);
  useEffect(() => { reload(); }, [reload]);
  useEffect(() => { if (staff) api("/catalog").then(setCatalog).catch(() => {}); }, [staff]);

  if (error) return <p className="error">{error === "project not found" ? "This project does not exist or you do not have access to it." : error}</p>;
  if (!data) return <p className="muted">Loading…</p>;
  const p = data.project;

  return (
    <div className="stack">
      <div>
        <p className="small"><Link href="/projects">← {staff ? "Project queue" : "My projects"}</Link></p>
        <div className="spread">
          <div>
            <h1>{p.title}</h1>
            <p className="muted">
              {p.service === "audit" ? "Contractor estimate & BOQ audit" : "Drawing-to-BOQ cost estimate"} · {p.building_type}, {p.floors}
              {p.built_up_area_sqft ? `, ${p.built_up_area_sqft.toLocaleString("en-IN")} sq ft` : ""} · {p.city}{p.state ? `, ${p.state}` : ""}
              {p.quoted_total ? ` · contractor quoted ${inr(p.quoted_total)}` : ""}
              {staff ? ` · ${p.customer_name} (${p.customer_email})` : ""}
            </p>
          </div>
          <span className={`badge ${p.status === "delivered" ? "ok" : "brand"}`}>{STATUS_LABEL[p.status] ?? p.status}</span>
        </div>
        {p.notes && <p className="small card" style={{ whiteSpace: "pre-wrap" }}>{p.notes}</p>}
      </div>

      {staff && (
        <div className="tabs" role="tablist">
          {TABS.map(([k, label]) => (
            <button key={k} role="tab" className="tab" aria-selected={tab === k} onClick={() => setTab(k)}>{label}</button>
          ))}
        </div>
      )}
      {(!staff || tab === "overview") && <Overview data={data} staff={staff} reload={reload} />}
      {staff && tab !== "overview" && !catalog && <p className="muted">Loading…</p>}
      {staff && catalog && tab === "boq" && <BoqTab project={p} catalog={catalog} reload={reload} />}
      {staff && catalog && tab === "takeoff" && <TakeoffTab project={p} catalog={catalog} documents={data.documents} />}
      {staff && tab === "analysis" && <AnalysisTab project={p} reload={reload} />}
    </div>
  );
}

export default function Page() {
  return <RequireUser>{(u) => <Project user={u} />}</RequireUser>;
}
