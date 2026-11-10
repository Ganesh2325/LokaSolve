"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Card } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Issue = {
  id: string;
  description: string;
  category_label: string | null;
  stated_urgency: string;
  status: string;
  address_text: string | null;
  locality_text: string | null;
  created_at: string;
};

export default function ReportsPage() {
  const { token } = useAuth();
  const [issues, setIssues] = useState<Issue[] | null>(null);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");

  useEffect(() => {
    if (!token) return;
    const handle = window.setTimeout(() => {
      const path = query.trim() ? `/api/v1/issues?q=${encodeURIComponent(query.trim())}` : "/api/v1/issues";
      api<{ data: Issue[] }>(path, { token })
        .then((response) => setIssues(response.data))
        .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load reports."));
    }, 250);
    return () => window.clearTimeout(handle);
  }, [token, query]);

  return (
    <div className="desk">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="kicker">Your submissions</p>
          <h1>My reports</h1>
        </div>
        <Link href="/reports/new" className="btn btn-gold">
          Report a problem
        </Link>
      </header>
      <input className="min-h-12 w-full rounded-lg border border-black bg-card px-3" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search your reports" aria-label="Search your reports" />
      {error ? <Card><p>{error}</p></Card> : null}
      {issues && issues.length === 0 ? (
        <Card>
          <p>No reports yet. A report you file stays yours. It is not treated as an accepted job.</p>
        </Card>
      ) : null}
      <ul className="grid gap-3">
        {issues?.map((issue) => (
          <li key={issue.id}>
            <Link href={`/reports/${issue.id}`} className="block border border-black bg-card px-4 py-3">
              <span className="text-xs font-bold uppercase tracking-[0.12em]">{issue.status.replaceAll("_", " ")}</span>
              <h2 className="mt-1 font-semibold">{issue.category_label || "Unlabeled problem"}</h2>
              <p className="mt-1 line-clamp-2 text-sm">{issue.description}</p>
              <p className="mt-2 text-sm text-muted">{[issue.locality_text, issue.address_text].filter(Boolean).join(" · ") || "No place written down"}</p>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
