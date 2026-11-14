"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { Button, Card, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Incident = {
  incident_id: string;
  incident_number?: string;
  title?: string;
  summary?: string;
  state?: string;
  severity?: string;
  public_report_count?: number;
  affected_area?: string;
  dispatched?: boolean;
};

type EventItem = { event_type: string; version: number; at: string; previous_state: string; new_state: string };

export default function IncidentDetailPage() {
  const params = useParams<{ id: string }>();
  const { token, user } = useAuth();
  const [item, setItem] = useState<Incident | null>(null);
  const [history, setHistory] = useState<EventItem[]>([]);
  const [source, setSource] = useState("");
  const [issue, setIssue] = useState("");
  const [error, setError] = useState("");
  const reviewer = user?.roles.includes("super_admin") ?? false;

  function load() {
    if (!token || !params.id) return;
    api<Incident>(`/api/v1/incidents/${params.id}`, { token }).then(setItem).catch((caught) => setError(caught instanceof ApiError ? caught.message : "That incident is not public."));
    api<{ data: EventItem[] }>(`/api/v1/incidents/${params.id}/history`, { token }).then((response) => setHistory(response.data)).catch(() => setHistory([]));
  }

  useEffect(() => { load(); }, [token, params.id]);

  return (
    <div className="flex flex-col gap-4">
      <Link href="/incidents" className="text-sm font-semibold underline">All incidents</Link>
      <h1 className="font-serif text-4xl">{item?.title || item?.incident_number || "Incident"}</h1>
      {error ? <ErrorText error={error} /> : null}
      {item ? (
        <Card>
          <p className="text-sm">{item.summary}</p>
          <p className="mt-2 text-sm">{item.incident_number} · {item.state} · {item.severity} · {item.public_report_count ?? 0} public reports</p>
          <p className="mt-2 text-sm">{item.affected_area}</p>
          <p className="mt-2 text-sm">Dispatched: no. Authority acknowledgement: no.</p>
        </Card>
      ) : null}
      <Card>
        <h2 className="font-serif text-2xl">Timeline</h2>
        {history.length === 0 ? <p className="mt-2 text-sm">No public timeline entries.</p> : null}
        <ul className="mt-3 flex flex-col gap-2 text-sm">
          {history.map((entry, index) => (
            <li key={`${entry.event_type}-${index}`}>{entry.new_state || entry.event_type} · {entry.at}</li>
          ))}
        </ul>
      </Card>
      {reviewer ? (
        <Card>
          <h2 className="font-serif text-2xl">Reviewer actions</h2>
          <p className="mt-2 text-sm">Closing an incident does not close the resident reports.</p>
          <div className="mt-3 flex flex-wrap gap-2">
            {["OPEN", "MONITORING", "RESOLUTION_PENDING", "CLOSED", "REOPENED"].map((state) => (
              <Button key={state} onClick={() => api(`/api/v1/incidents/${params.id}/transition`, { method: "POST", token, body: { state } }).then(load).catch((caught) => setError(caught instanceof ApiError ? caught.message : "That change was not allowed."))}>{state}</Button>
            ))}
          </div>
          <form className="mt-4 flex flex-col gap-2" onSubmit={(event) => { event.preventDefault(); api(`/api/v1/incidents/${params.id}/merge`, { method: "POST", token, body: { source_id: source } }).then(load).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not merge.")); }}>
            <Field label="Incident to merge into this one">
              <input className={controlClass} value={source} onChange={(event) => setSource(event.target.value)} />
            </Field>
            <Button type="submit">Merge</Button>
          </form>
          <form className="mt-4 flex flex-col gap-2" onSubmit={(event) => { event.preventDefault(); api(`/api/v1/incidents/${params.id}/split`, { method: "POST", token, body: { issue_id: issue } }).then(load).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not split.")); }}>
            <Field label="Report to separate">
              <input className={controlClass} value={issue} onChange={(event) => setIssue(event.target.value)} />
            </Field>
            <Button type="submit">Split</Button>
          </form>
        </Card>
      ) : null}
    </div>
  );
}
