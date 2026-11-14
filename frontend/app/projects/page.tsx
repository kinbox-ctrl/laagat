"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { api, date, inr, isStaff, STATUS_LABEL, User } from "@/lib/api";
import { RequireUser } from "@/components/RequireUser";

function List({ user }: { user: User }) {
  const [rows, setRows] = useState<any[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api("/projects").then(setRows).catch((e) => setError(e.message));
  }, []);
  const staff = isStaff(user);
  return (
    <div className="stack">
      <div className="spread">
        <div>
          <h1>{staff ? "Project queue" : "My projects"}</h1>
          <p className="muted">{staff ? "All customer projects, newest first." : "Track your assessments and download reports."}</p>
        </div>
        {!staff && <Link href="/projects/new" className="btn">New project</Link>}
      </div>
      {error && <p className="error">{error}</p>}
      {rows === null && !error && <p className="muted">Loading…</p>}
      {rows?.length === 0 && (
        <div className="card">
          <p>{staff ? "No projects have been submitted yet." : "You have no projects yet."}</p>
          {!staff && <Link href="/projects/new" className="btn">Start your first project</Link>}
        </div>
      )}
      {!!rows?.length && (
        <div className="card flush table-wrap">
          <table>
            <thead>
              <tr>
                <th>Project</th>{staff && <th>Customer</th>}<th>Service</th><th>Location</th><th>Status</th>
                <th className="num">Quoted by contractor</th><th>Submitted</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((p) => (
                <tr key={p.id}>
                  <td><Link href={`/projects/${p.id}`}><strong>{p.title}</strong></Link></td>
                  {staff && <td>{p.customer_name}</td>}
                  <td>{p.service === "audit" ? "Estimate audit" : "Cost estimate"}</td>
                  <td>{p.city}</td>
                  <td><span className={`badge ${p.status === "delivered" ? "ok" : "brand"}`}>{STATUS_LABEL[p.status] ?? p.status}</span></td>
                  <td className="num">{p.quoted_total ? inr(p.quoted_total) : "—"}</td>
                  <td>{date(p.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

export default function Page() {
  return <RequireUser>{(u) => <List user={u} />}</RequireUser>;
}
