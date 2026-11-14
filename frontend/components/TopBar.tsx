"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { api, isStaff } from "@/lib/api";
import { useSession } from "@/lib/session";

export function TopBar() {
  const { user, loading, reload } = useSession();
  const router = useRouter();
  const logout = async () => {
    await api("/auth/logout", { method: "POST" }).catch(() => {});
    await reload();
    router.push("/");
  };
  return (
    <header className="topbar">
      <div className="topbar-inner">
        <Link href="/" className="logo">
          BUILDCOST<span>CONSTRUCTION COST INTELLIGENCE</span>
        </Link>
        <nav className="nav">
          {loading ? null : user ? (
            <>
              <Link href="/projects">{isStaff(user) ? "Project queue" : "My projects"}</Link>
              {isStaff(user) && <Link href="/rates">Rates &amp; rules</Link>}
              <span className="muted small">{user.name}</span>
              <button className="btn secondary sm" onClick={logout}>Sign out</button>
            </>
          ) : (
            <>
              <Link href="/login">Sign in</Link>
              <Link href="/register" className="btn sm">Start a project</Link>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
