"use client";
import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { api, User } from "./api";

type Session = { user: User | null; aiEnabled: boolean; loading: boolean; reload: () => Promise<void> };
const Ctx = createContext<Session>({ user: null, aiEnabled: false, loading: true, reload: async () => {} });

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<Omit<Session, "reload">>({ user: null, aiEnabled: false, loading: true });
  const reload = useCallback(async () => {
    try {
      const me = await api("/me");
      setState({ user: me.user, aiEnabled: me.ai_enabled, loading: false });
    } catch {
      setState({ user: null, aiEnabled: false, loading: false });
    }
  }, []);
  useEffect(() => {
    reload();
  }, [reload]);
  return <Ctx.Provider value={{ ...state, reload }}>{children}</Ctx.Provider>;
}

export const useSession = () => useContext(Ctx);

/** Runs an async action and tracks its pending and error state. */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const run = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "Something went wrong");
      return false;
    } finally {
      setBusy(false);
    }
  }, []);
  return { busy, error, run, setError };
}
