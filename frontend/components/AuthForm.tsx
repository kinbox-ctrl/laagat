"use client";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { api } from "@/lib/api";
import { useAction, useSession } from "@/lib/session";

export function AuthForm({ mode }: { mode: "login" | "register" }) {
  const router = useRouter();
  const next = useSearchParams().get("next") || "/projects";
  const { reload } = useSession();
  const { busy, error, run } = useAction();
  const [form, setForm] = useState({ name: "", email: "", password: "" });
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value });

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    run(async () => {
      await api(`/auth/${mode}`, { body: form });
      await reload();
      router.push(next.startsWith("/") ? next : "/projects");
    });
  };
  const register = mode === "register";
  return (
    <div className="narrow card">
      <h1>{register ? "Create your account" : "Sign in"}</h1>
      <p className="muted">{register ? "Then tell us about your project." : "Welcome back."}</p>
      <form onSubmit={submit}>
        {register && (
          <div className="field">
            <label htmlFor="name">Your name</label>
            <input id="name" value={form.name} onChange={set("name")} autoComplete="name" required />
          </div>
        )}
        <div className="field">
          <label htmlFor="email">Email</label>
          <input id="email" type="email" value={form.email} onChange={set("email")} autoComplete="email" required />
        </div>
        <div className="field">
          <label htmlFor="password">Password</label>
          <input id="password" type="password" value={form.password} onChange={set("password")} minLength={register ? 8 : undefined}
            autoComplete={register ? "new-password" : "current-password"} required />
          {register && <div className="hint">At least 8 characters.</div>}
        </div>
        {error && <p className="error" role="alert">{error}</p>}
        <button className="btn" disabled={busy}>{busy ? "Please wait…" : register ? "Create account" : "Sign in"}</button>
      </form>
      <p className="small muted" style={{ marginTop: 14 }}>
        {register ? <>Already have an account? <Link href={`/login?next=${encodeURIComponent(next)}`}>Sign in</Link></>
          : <>New here? <Link href={`/register?next=${encodeURIComponent(next)}`}>Create an account</Link></>}
      </p>
    </div>
  );
}
