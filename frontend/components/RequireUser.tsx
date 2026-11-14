"use client";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { isStaff, User } from "@/lib/api";
import { useSession } from "@/lib/session";

/** Renders children once a user is signed in; otherwise redirects to sign-in. */
export function RequireUser({ staff, children }: { staff?: boolean; children: (user: User) => React.ReactNode }) {
  const { user, loading } = useSession();
  const router = useRouter();
  const path = usePathname();
  useEffect(() => {
    if (!loading && !user) router.replace(`/login?next=${encodeURIComponent(path)}`);
  }, [loading, user, router, path]);
  if (loading || !user) return <p className="muted">Loading…</p>;
  if (staff && !isStaff(user)) return <p className="error">This page is for BuildCost staff.</p>;
  return <>{children(user)}</>;
}
