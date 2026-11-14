export async function api<T = any>(path: string, opts: { method?: string; body?: unknown; form?: FormData } = {}): Promise<T> {
  const init: RequestInit = { method: opts.method ?? (opts.body || opts.form ? "POST" : "GET"), credentials: "same-origin" };
  if (opts.form) init.body = opts.form;
  else if (opts.body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(opts.body);
  }
  let res: Response;
  try {
    res = await fetch("/api" + path, init);
  } catch {
    throw new Error("Could not reach the server. Check your connection and try again.");
  }
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new Error(data?.error ?? `Request failed (${res.status})`);
  return data as T;
}

export const inr = (v: number) => "₹" + Math.round(v ?? 0).toLocaleString("en-IN");
export const num = (v: number, places = 2) => (v ?? 0).toLocaleString("en-IN", { maximumFractionDigits: places });
export const date = (s: string) => new Date(s).toLocaleDateString("en-IN", { day: "numeric", month: "short", year: "numeric" });

export const STATUS_LABEL: Record<string, string> = {
  submitted: "Submitted",
  quoted: "Quotation sent",
  accepted: "Quotation accepted",
  paid: "Paid",
  in_analysis: "In analysis",
  in_review: "In review",
  approved: "Report approved",
  delivered: "Delivered",
};

export const DOC_KINDS: Record<string, string> = {
  quotation: "Contractor quotation",
  boq: "BOQ",
  drawing_architectural: "Architectural drawing",
  drawing_structural: "Structural drawing",
  specification: "Specification",
  other: "Other",
};

export type User = { id: number; name: string; email: string; role: string };
export const isStaff = (u: User | null) => !!u && u.role !== "customer";
