"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { Button, Card, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Issue = {
  id: string;
  description: string;
  category_label: string | null;
  stated_urgency: string;
  address_text: string | null;
  locality_text: string | null;
  latitude: number | null;
  longitude: number | null;
  status: string;
  answers: Record<string, string>;
};

type TimelineItem = { id: string; kind: string; body: string; created_at: string };
type MediaItem = { id: string; file_name: string };
type Eligible = { id: string; display_name: string; explanation: string; verification_status: string };
type Assignment = {
  id: string;
  issue_id: string;
  resolver_name: string;
  status: string;
  explanation: string;
  quote_text: string | null;
  quote_approved: boolean;
  can_resolve: boolean;
  can_confirm: boolean;
};
type Office = { id: string; official_name: string; verification_status: string; explanation: string };
type Referral = { id: string; authority_name: string; status: string; channel: string; external_reference: string | null };

export default function ReportDetailPage() {
  const params = useParams<{ id: string }>();
  const { token, user } = useAuth();
  const [issue, setIssue] = useState<Issue | null>(null);
  const [timeline, setTimeline] = useState<TimelineItem[]>([]);
  const [media, setMedia] = useState<MediaItem[]>([]);
  const [eligible, setEligible] = useState<Eligible[] | null>(null);
  const [offices, setOffices] = useState<Office[] | null>(null);
  const [referrals, setReferrals] = useState<Referral[]>([]);
  const [consent, setConsent] = useState(false);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [label, setLabel] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const canOperate = user?.roles.includes("super_admin") ?? false;

  function reload() {
    if (!token || !params.id) return;
    api<Issue>(`/api/v1/issues/${params.id}`, { token }).then(setIssue).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not open the report."));
    api<{ data: TimelineItem[] }>(`/api/v1/issues/${params.id}/timeline`, { token }).then((response) => setTimeline(response.data)).catch(() => setTimeline([]));
    api<{ data: MediaItem[] }>(`/api/v1/issues/${params.id}/media`, { token }).then((response) => setMedia(response.data)).catch(() => setMedia([]));
    api<{ data: Assignment[] }>("/api/v1/assignments", { token }).then((response) => setAssignments(response.data.filter((item) => item.issue_id === params.id))).catch(() => setAssignments([]));
    api<{ data: Referral[] }>(`/api/v1/issues/${params.id}/referrals`, { token }).then((response) => setReferrals(response.data)).catch(() => setReferrals([]));
  }

  useEffect(() => {
    reload();
  }, [token, params.id]);

  async function upload(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file || !token) return;
    const body = new FormData();
    body.set("file", file);
    setError("");
    const response = await fetch(`/api/v1/issues/${params.id}/media`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "X-LokaSolve-Client": "web" },
      body,
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => ({}));
      setError(payload.error?.message ?? "Could not add the photo.");
      return;
    }
    reload();
  }

  async function saveLabel(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await api(`/api/v1/issues/${params.id}/classification`, { method: "POST", token, body: { category_label: label } });
      setLabel("");
      reload();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not save the label.");
    }
  }

  async function act(path: string, body?: unknown) {
    setError("");
    try {
      await api(path, { method: "POST", token, body });
      reload();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not update the report.");
    }
  }

  if (error && !issue) return <Card><p>{error}</p></Card>;
  if (!issue) return <p>Loading the report…</p>;
  const place = [issue.locality_text, issue.address_text].filter(Boolean).join(" · ");

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
      <section className="flex flex-col gap-3">
        <header className="flex items-start justify-between gap-3">
          <h1 className="font-serif text-3xl">{issue.category_label || "Unlabeled problem"}</h1>
          <span className="shrink-0 border border-black px-2 py-1 text-xs font-bold uppercase">{issue.status.replaceAll("_", " ")}</span>
        </header>
        <p>{issue.description}</p>
        <p className="text-sm">{place || "No place written down"} · {issue.latitude != null ? `${issue.latitude}, ${issue.longitude}` : "No coordinates stored"}</p>
        {error ? <ErrorText error={error} /> : null}
        {Object.entries(issue.answers ?? {}).filter(([, value]) => value).map(([key, value]) => (
          <p key={key} className="text-sm"><span className="font-semibold">{key}. </span>{value}</p>
        ))}
        <Card>
          <h2 className="font-serif text-2xl">Photos</h2>
          <input className="mt-3 block text-sm" type="file" accept="image/jpeg,image/png,image/webp,image/gif,audio/webm,audio/mpeg,video/mp4,video/webm" onChange={upload} />
          <div className="mt-3 grid grid-cols-2 gap-2">
            {media.map((item) => token ? <Photo key={item.id} issueId={issue.id} mediaId={item.id} token={token} /> : null)}
          </div>
        </Card>
        <form className="flex gap-2" onSubmit={saveLabel}>
          <input className={controlClass} value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Set a label" aria-label="Label" />
          <Button type="submit">Save</Button>
        </form>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => act(`/api/v1/issues/${issue.id}/incident`)}>Open incident</Button>
          <Button onClick={() => api<{ data: Eligible[] }>(`/api/v1/issues/${issue.id}/eligible`, { token }).then((response) => setEligible(response.data)).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not check responders."))}>Check responders</Button>
          <Button onClick={() => api<{ data: Office[] }>(`/api/v1/issues/${issue.id}/authorities`, { token }).then((response) => setOffices(response.data)).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not check offices."))}>Check offices</Button>
        </div>
        {offices ? (
          <Card>
            {offices.length === 0 ? <p className="text-sm">No office on file matches this place or label. Nothing was sent.</p> : null}
            <ul className="flex flex-col gap-3">
              {offices.map((office) => (
                <li key={office.id}>
                  <p className="font-semibold">{office.official_name}</p>
                  <p className="text-sm">{office.explanation}</p>
                  <label className="mt-2 flex items-start gap-2 text-sm">
                    <input type="checkbox" checked={consent} onChange={(event) => setConsent(event.target.checked)} />
                    <span>I agree to share this description and place. My phone number stays off the packet.</span>
                  </label>
                  <Button className="mt-2" onClick={() => act(`/api/v1/issues/${issue.id}/referrals`, { authority_id: office.id, consent })}>Prepare packet</Button>
                </li>
              ))}
            </ul>
          </Card>
        ) : null}
        {referrals.map((item) => (
          <Card key={item.id}>
            <p className="font-semibold">{item.authority_name}</p>
            <p className="mt-1 text-sm uppercase">{item.status.replaceAll("_", " ")} · {item.channel}</p>
            {item.external_reference ? <p className="mt-1 text-sm">Reference {item.external_reference}</p> : null}
            <div className="mt-3 flex flex-wrap gap-2">
              {canOperate && item.status === "consented" ? <Button onClick={() => act(`/api/v1/referrals/${item.id}/advance`, { status: "approved_for_submission", note })}>Approve packet</Button> : null}
              {canOperate && item.status === "approved_for_submission" ? <Button onClick={() => act(`/api/v1/referrals/${item.id}/advance`, { status: "manual_handoff", note })}>Manual handoff</Button> : null}
              {canOperate && (item.status === "manual_handoff" || item.status === "not_supported") ? <Button onClick={() => act(`/api/v1/referrals/${item.id}/advance`, { status: "acknowledged", note })}>Record acknowledgement</Button> : null}
              {canOperate && item.status === "acknowledged" ? <Button onClick={() => act(`/api/v1/referrals/${item.id}/advance`, { status: "officially_registered", note, external_reference: note })}>Record reference</Button> : null}
              {item.status === "resolution_reported" ? <Button onClick={() => act(`/api/v1/referrals/${item.id}/advance`, { status: "confirmed", note: note || "Confirmed" })}>Confirm</Button> : null}
              {item.status === "resolution_reported" ? <Button onClick={() => act(`/api/v1/referrals/${item.id}/advance`, { status: "disputed", note })}>Dispute</Button> : null}
              {canOperate ? <Button onClick={() => act(`/api/v1/referrals/${item.id}/escalate`)}>Queue escalation</Button> : null}
            </div>
          </Card>
        ))}
        {eligible ? (
          <Card>
            {eligible.length === 0 ? <p className="text-sm">No responder on file matches this place or label.</p> : null}
            <ul className="flex flex-col gap-3">
              {eligible.map((item) => (
                <li key={item.id}>
                  <p className="font-semibold">{item.display_name}</p>
                  <p className="text-sm">{item.explanation}</p>
                  {canOperate ? <Button className="mt-2" onClick={() => act(`/api/v1/issues/${issue.id}/assignments`, { resolver_id: item.id })}>Record offer</Button> : null}
                </li>
              ))}
            </ul>
          </Card>
        ) : null}
        {assignments.map((item) => (
          <Card key={item.id}>
            <p className="font-semibold">{item.resolver_name} · {item.status.replaceAll("_", " ")}</p>
            <p className="mt-1 text-sm">{item.explanation}</p>
            {item.quote_text ? <p className="mt-2 text-sm">Quote: {item.quote_text}{item.quote_approved ? " · approved" : ""}</p> : null}
            <div className="mt-3 flex flex-wrap gap-2">
              {item.can_resolve && item.status === "offered" ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "accepted", note })}>Accept</Button> : null}
              {item.can_resolve && item.status === "offered" ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "rejected", note })}>Decline</Button> : null}
              {item.can_resolve && (item.status === "accepted" || item.status === "scheduled") ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "in_progress", note })}>Start</Button> : null}
              {item.can_resolve && item.status === "accepted" ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "scheduled", note })}>Schedule</Button> : null}
              {item.can_resolve && item.status === "in_progress" ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "resolution_submitted", note })}>Submit evidence</Button> : null}
              {item.can_confirm && item.quote_text && !item.quote_approved ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/approve-quote`)}>Approve quote</Button> : null}
              {item.can_confirm && item.status === "resolution_submitted" ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "resolved", note })}>Confirm fixed</Button> : null}
              {item.can_confirm && item.status === "resolution_submitted" ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "disputed", note })}>Dispute</Button> : null}
              {canOperate && item.status === "disputed" ? <Button onClick={() => act(`/api/v1/assignments/${item.id}/transition`, { status: "reopened", note })}>Send back</Button> : null}
            </div>
          </Card>
        ))}
        {assignments.some((item) => item.can_resolve || item.can_confirm) ? (
          <Field label="Note for the next step">
            <input className={controlClass} value={note} onChange={(event) => setNote(event.target.value)} />
          </Field>
        ) : null}
        <Link href="/reports" className="text-sm font-semibold underline">Back to my reports</Link>
      </section>
      <section>
        <h2 className="font-serif text-2xl">Timeline</h2>
        <ol className="mt-3 flex flex-col gap-3">
          {timeline.map((item) => (
            <li key={item.id} className="border-l-4 border-[#c4a15a] bg-card px-3 py-2">
              <p className="text-xs font-bold uppercase">{item.kind}</p>
              <p className="mt-1 text-sm">{item.body}</p>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}

function Photo({ issueId, mediaId, token }: { issueId: string; mediaId: string; token: string }) {
  const [src, setSrc] = useState("");
  useEffect(() => {
    let url = "";
    let active = true;
    fetch(`/api/v1/issues/${issueId}/media/${mediaId}`, { headers: { Authorization: `Bearer ${token}`, "X-LokaSolve-Client": "web" } })
      .then((response) => response.blob())
      .then((blob) => {
        if (!active) return;
        url = URL.createObjectURL(blob);
        setSrc(url);
      })
      .catch(() => setSrc(""));
    return () => {
      active = false;
      if (url) URL.revokeObjectURL(url);
    };
  }, [issueId, mediaId, token]);
  if (!src) return null;
  return <img src={src} alt="" className="h-28 w-full border border-black object-cover" />;
}
