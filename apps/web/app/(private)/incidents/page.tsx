"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Button, Card, ErrorText } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Incident = {
  incident_id: string;
  incident_number: string;
  title: string;
  summary: string;
  state: string;
  severity: string;
  public_report_count: number;
};

type Side = { category: string; severity: string; public: boolean; validated: boolean };
type Candidate = {
  id: string;
  left_issue_id: string;
  right_issue_id: string;
  score: number;
  reasoning: string;
  interpretation: string;
  distance_m: number | null;
  age_seconds: number;
  review_status: string;
  left: Side;
  right: Side;
};

export default function IncidentsPage() {
  const { token, user } = useAuth();
  const [items, setItems] = useState<Incident[] | null>(null);
  const [candidates, setCandidates] = useState<Candidate[] | null>(null);
  const [error, setError] = useState("");
  const permissions = user?.permissions ?? [];
  const reviewer = permissions.includes("incident.candidate.review") || (user?.roles.includes("super_admin") ?? false) || (user?.roles.includes("incident_analyst") ?? false) || (user?.roles.includes("operations_reviewer") ?? false) || (user?.roles.includes("incident_supervisor") ?? false);
  const canAssociate = permissions.includes("incident.associate") || (user?.roles.includes("super_admin") ?? false) || (user?.roles.includes("operations_reviewer") ?? false) || (user?.roles.includes("incident_supervisor") ?? false);

  useEffect(() => {
    if (!token) return;
    api<{ data: Incident[] }>("/api/v1/incidents", { token })
      .then((response) => setItems(response.data))
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load incidents."));
    if (!reviewer) return;
    api<{ data: Candidate[] }>("/api/v1/incidents/candidates", { token })
      .then((response) => setCandidates(response.data))
      .catch(() => setCandidates([]));
  }, [token, reviewer]);

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-serif text-4xl">Incidents</h1>
      <p className="text-sm">Public shared problems only. A private report is not listed here, and nobody has been dispatched.</p>
      {error ? <ErrorText error={error} /> : null}
      {items && items.length === 0 ? <Card><p className="text-sm">No public incidents yet.</p></Card> : null}
      {items?.map((item) => (
        <Card key={item.incident_id}>
          <h2 className="font-serif text-2xl">{item.title}</h2>
          <p className="mt-2 text-sm">{item.summary}</p>
          <p className="mt-2 text-sm">{item.incident_number} · {item.state} · {item.severity} · {item.public_report_count} public reports</p>
          <Link className="mt-2 inline-block text-sm font-semibold underline" href={`/incidents/${item.incident_id}`}>Details</Link>
        </Card>
      ))}
      {reviewer ? (
        <Card>
          <h2 className="font-serif text-2xl">Duplicate review</h2>
          {candidates && candidates.length === 0 ? <p className="mt-2 text-sm">No candidates waiting.</p> : null}
          <ul className="mt-3 flex flex-col gap-3">
            {candidates?.map((item) => (
              <li key={item.id} className="border-t border-black/10 pt-3 text-sm">
                <p>Score {item.score.toFixed(2)} · {item.interpretation}</p>
                <p className="mt-1">{item.distance_m != null ? `${Math.round(item.distance_m)} m` : "No shared coordinates"} · {Math.round((item.age_seconds ?? 0) / 60)} minutes apart · {item.review_status}</p>
                <p className="mt-1">{item.left.category || "Unchecked"} {item.left.severity} · {item.left.public ? "public" : "private"} and {item.right.category || "Unchecked"} {item.right.severity} · {item.right.public ? "public" : "private"}</p>
                <p className="mt-1">{item.reasoning}</p>
                <div className="mt-2 flex flex-wrap gap-2">
                  {canAssociate ? <Button onClick={() => api(`/api/v1/incident-candidates/${item.id}`, { method: "POST", token, body: { accept: true } }).then(() => setCandidates((current) => current?.filter((row) => row.id !== item.id) ?? [])).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not accept."))}>Approve link</Button> : null}
                  <Button onClick={() => api(`/api/v1/incident-candidates/${item.id}`, { method: "POST", token, body: { accept: false } }).then(() => setCandidates((current) => current?.filter((row) => row.id !== item.id) ?? [])).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not reject."))}>Reject</Button>
                  <Button onClick={() => api(`/api/v1/incident-candidates/${item.id}/review`, { method: "POST", token, body: {} }).then(() => setCandidates((current) => current?.map((row) => row.id === item.id ? { ...row, review_status: "more_review" } : row) ?? [])).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not request review."))}>More review</Button>
                </div>
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
    </div>
  );
}
