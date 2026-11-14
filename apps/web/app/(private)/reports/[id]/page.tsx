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
  classification_provenance?: string;
  suggested_label?: string | null;
  stated_urgency: string;
  address_text: string | null;
  locality_text: string | null;
  latitude: number | null;
  longitude: number | null;
  follow_public?: boolean;
  version?: number;
  coordinates_removed?: boolean;
  location_sync?: string;
  status: string;
  answers: Record<string, string>;
  severity?: { level: string; source: string; reason: string; overridden: boolean };
  safety_guidance?: string;
  escalation_due_at?: string;
};

type ProgressStep = { key: string; title: string; state: string; detail: string; at?: string };
type Progress = {
  where: string;
  responsible: string;
  packet_prepared: boolean;
  submitted: boolean;
  delivered: boolean;
  officially_acknowledged: boolean;
  official_reference: string | null;
  operator_reference: string | null;
  reference_explanation: string;
  next_actor: string;
  escalation_due_at: string | null;
  if_nobody_responds: string;
  steps: ProgressStep[];
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
  const [progress, setProgress] = useState<Progress | null>(null);
  const [progressState, setProgressState] = useState<"loading" | "ready" | "missing">("loading");
  const [consent, setConsent] = useState(false);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [lat, setLat] = useState("");
  const [lng, setLng] = useState("");
  const [share, setShare] = useState(false);
  const [shareConsent, setShareConsent] = useState(false);
  const [removeConsent, setRemoveConsent] = useState(false);
  const [label, setLabel] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [incident, setIncident] = useState<{ linked?: boolean; public?: boolean; incident_id?: string; incident_number?: string; summary?: string; state?: string; severity?: string; public_report_count?: number } | null>(null);
  const canOperate = user?.roles.includes("super_admin") ?? false;

  function reload() {
    if (!token || !params.id) return;
    api<Issue>(`/api/v1/issues/${params.id}`, { token }).then(setIssue).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not open the report."));
    api<{ data: TimelineItem[] }>(`/api/v1/issues/${params.id}/timeline`, { token }).then((response) => setTimeline(response.data)).catch(() => setTimeline([]));
    api<{ data: MediaItem[] }>(`/api/v1/issues/${params.id}/media`, { token }).then((response) => setMedia(response.data)).catch(() => setMedia([]));
    api<{ data: Assignment[] }>("/api/v1/assignments", { token }).then((response) => setAssignments(response.data.filter((item) => item.issue_id === params.id))).catch(() => setAssignments([]));
    api<{ data: Referral[] }>(`/api/v1/issues/${params.id}/referrals`, { token }).then((response) => setReferrals(response.data)).catch(() => setReferrals([]));
    api<{ linked?: boolean; public?: boolean; incident_id?: string; incident_number?: string; summary?: string; state?: string; severity?: string; public_report_count?: number }>(`/api/v1/incident-membership/${params.id}`, { token }).then(setIncident).catch(() => setIncident(null));
    setProgressState("loading");
    api<Progress>(`/api/v1/issues/${params.id}/progress`, { token }).then((item) => {
      setProgress(item);
      setProgressState("ready");
    }).catch(() => {
      setProgress(null);
      setProgressState("missing");
    });
  }

  useEffect(() => {
    reload();
  }, [token, params.id]);

  useEffect(() => {
    if (!issue) return;
    setLat(issue.latitude == null ? "" : String(issue.latitude));
    setLng(issue.longitude == null ? "" : String(issue.longitude));
    setShare(Boolean(issue.follow_public));
  }, [issue]);

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
          <span className="shrink-0 border border-black px-2 py-1 text-xs font-bold uppercase">{issue.severity?.level ?? issue.status.replaceAll("_", " ")}</span>
        </header>
        <p>{issue.description}</p>
        {issue.suggested_label ? (
          <p className="text-sm">Suggested label: {issue.suggested_label}. {issue.classification_provenance === "AI_SUGGESTED" ? "A model suggested this." : "A keyword suggested this."} It is not a confirmed category and it does not choose an office or a provider.</p>
        ) : null}
        {issue.classification_provenance === "USER_SELECTED_VALIDATED" || issue.classification_provenance === "HUMAN_VERIFIED" ? (
          <p className="text-sm">The category was {issue.classification_provenance === "HUMAN_VERIFIED" ? "verified by an operator" : "checked against the published list"}. That is not proof of which government office has jurisdiction.</p>
        ) : null}
        <p className="text-sm">{place || "No place written down"} · {issue.coordinates_removed ? "Precise coordinates removed" : issue.latitude != null ? "A precise point is saved for you" : "No coordinates stored"}</p>
        <p className="text-sm">{issue.location_sync === "pending" ? "The change is saved on the report and is waiting to reach the map service." : "The map service has the latest saved place. Private reports are not listed publicly, and exact coordinates are not shown on a public page."}</p>
        <Card>
          <h2 className="font-serif text-2xl">Place and sharing</h2>
          <p className="mt-2 text-sm">Only you see the numbers below. Correcting them does not change the description or the status. Removing the precise point can make a ward match or a visit to the exact place harder. It does not delete the report, and it does not erase every stored copy of personal data.</p>
          <form className="mt-3 grid gap-3" onSubmit={(event) => {
            event.preventDefault();
            setError("");
            api(`/api/v1/issues/${issue.id}/location`, { method: "PATCH", token, body: { latitude: Number(lat), longitude: Number(lng), expected_version: issue.version ?? 0 } })
              .then(() => reload())
              .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not save the place."));
          }}>
            <Field label="Latitude">
              <input className={controlClass} value={lat} onChange={(event) => setLat(event.target.value)} inputMode="decimal" aria-label="Latitude" />
            </Field>
            <Field label="Longitude">
              <input className={controlClass} value={lng} onChange={(event) => setLng(event.target.value)} inputMode="decimal" aria-label="Longitude" />
            </Field>
            <Button type="submit">Save corrected place</Button>
          </form>
          <div className="mt-4 flex flex-col gap-2">
            <label className="flex items-start gap-2 text-sm">
              <input type="checkbox" checked={share} onChange={(event) => setShare(event.target.checked)} />
              <span>Allow this report on public lists. Exact coordinates stay off those lists.</span>
            </label>
            {share && !issue.follow_public ? (
              <label className="flex items-start gap-2 text-sm">
                <input type="checkbox" checked={shareConsent} onChange={(event) => setShareConsent(event.target.checked)} />
                <span>I agree to make this report public.</span>
              </label>
            ) : null}
            <Button onClick={() => {
              setError("");
              api(`/api/v1/issues/${issue.id}/visibility`, { method: "PATCH", token, body: { follow_public: share, consent: shareConsent || Boolean(issue.follow_public && share), expected_version: issue.version ?? 0 } })
                .then(() => reload())
                .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not change sharing."));
            }}>Save sharing</Button>
          </div>
          <div className="mt-4 border-t border-black/10 pt-3">
            <label className="flex items-start gap-2 text-sm">
              <input type="checkbox" checked={removeConsent} onChange={(event) => setRemoveConsent(event.target.checked)} />
              <span>Remove the precise coordinates. I understand the written report stays.</span>
            </label>
            <Button className="mt-2" onClick={() => act(`/api/v1/issues/${issue.id}/location/removal`, { confirm: removeConsent, expected_version: issue.version ?? 0 })}>Remove precise point</Button>
          </div>
        </Card>
        {issue.safety_guidance ? <Card><p className="text-sm">{issue.safety_guidance}</p></Card> : null}
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
        <Card>
          <h2 className="font-serif text-2xl">Shared incident</h2>
          {!incident?.linked ? <p className="mt-2 text-sm">This report is not part of a shared incident.</p> : null}
          {incident?.linked ? (
            <div className="mt-2 text-sm">
              <p>{incident.summary || "This report is linked to an incident. Other reporters are not shown."}</p>
              {incident.incident_number ? <p className="mt-1">{incident.incident_number} · {incident.state} · {incident.severity}</p> : null}
              {incident.public ? <p className="mt-1">{incident.public_report_count ?? 0} public reports. Exact private places are hidden.</p> : <p className="mt-1">This link is not on the public list.</p>}
              {incident.incident_id ? <Link className="mt-2 inline-block font-semibold underline" href={`/incidents/${incident.incident_id}`}>Incident timeline</Link> : null}
            </div>
          ) : null}
        </Card>
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
            {item.external_reference ? <p className="mt-1 text-sm">An operator entered {item.external_reference}. This is not a portal receipt.</p> : null}
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
        {canOperate || referrals.length > 0 ? (
          <Field label="Note for the next step">
            <input className={controlClass} value={note} onChange={(event) => setNote(event.target.value)} />
          </Field>
        ) : null}
        <Link href="/reports" className="text-sm font-semibold underline">Back to my reports</Link>
      </section>
      <section>
        <h2 className="font-serif text-2xl">Where this is</h2>
        {progress && progressState === "ready" ? (
          <div className="mt-3 flex flex-col gap-3">
            <Card>
              <p>{progress.where}</p>
              <p className="mt-2 text-sm">{progress.responsible}</p>
              <dl className="mt-3 grid gap-2 text-sm">
                <div><dt className="font-semibold">Packet prepared</dt><dd>{progress.packet_prepared ? "Yes. It has not been submitted by that fact alone." : "No."}</dd></div>
                <div><dt className="font-semibold">Submitted</dt><dd>{progress.submitted ? "A submission attempt is recorded." : "No submission has been attempted."}</dd></div>
                <div><dt className="font-semibold">Delivered</dt><dd>{progress.delivered ? "A delivery receipt is on file." : "No delivery receipt is on file."}</dd></div>
                <div><dt className="font-semibold">Official acknowledgement</dt><dd>{progress.officially_acknowledged ? "An official acknowledgement is recorded." : "None is on file."}</dd></div>
                <div><dt className="font-semibold">Official reference</dt><dd>{progress.official_reference ?? progress.reference_explanation}</dd></div>
                <div><dt className="font-semibold">Next action</dt><dd>{progress.next_actor}</dd></div>
                <div><dt className="font-semibold">Review again</dt><dd>{progress.escalation_due_at ? new Date(progress.escalation_due_at).toLocaleString() : "No deadline is recorded."}</dd></div>
              </dl>
              <p className="mt-3 text-sm">{progress.if_nobody_responds}</p>
            </Card>
            <ol className="flex flex-col gap-3">
              {progress.steps.map((item) => (
                <li key={item.key} className="border-l-4 border-[#c4a15a] bg-card px-3 py-2">
                  <p className="text-xs font-bold uppercase">{item.title} · {item.state === "recorded" ? "Recorded" : item.state === "waiting" ? "Waiting" : "Not recorded"}</p>
                  <p className="mt-1 text-sm">{item.detail}</p>
                </li>
              ))}
            </ol>
          </div>
        ) : <p className="mt-3 text-sm">{progressState === "missing" ? "The status could not be loaded." : "Loading the status…"}</p>}
        <h2 className="mt-6 font-serif text-2xl">Notes</h2>
        <ol className="mt-3 flex flex-col gap-3">
          {timeline.map((item) => (
            <li key={item.id} className="border-l-4 border-[#c4a15a] bg-card px-3 py-2">
              <p className="text-sm">{item.body}</p>
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
