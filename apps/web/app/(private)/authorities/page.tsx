"use client";

import { useEffect, useState } from "react";
import { Button, Card, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Authority = {
  id: string;
  official_name: string;
  entity_type: string;
  verification_state: string;
  routing_ready: boolean;
  notice?: string;
};

type Row = Record<string, string>;

const types = [
  "central_department", "state_department", "district_administration", "municipal_corporation",
  "municipality", "nagar_panchayat", "gram_panchayat", "public_utility", "electricity_distribution",
  "water_authority", "infrastructure_agency", "road_authority", "public_transport", "public_health",
  "pollution_control", "emergency_response", "grievance_redressal", "other_public_agency",
];

function badge(state: string) {
  const label = state === "PENDING_EVIDENCE" || state === "UNDER_REVIEW" ? "Pending verification" : state === "DRAFT" ? "Unverified" : state.charAt(0) + state.slice(1).toLowerCase();
  return <span className="rounded-full border border-black px-2 py-0.5 text-xs">{label}</span>;
}

export default function AuthoritiesPage() {
  const { token, user } = useAuth();
  const can = (code: string) => Boolean(user?.permissions?.includes(code) || user?.roles?.includes("super_admin"));
  const [rows, setRows] = useState<Authority[]>([]);
  const [selected, setSelected] = useState<Authority | null>(null);
  const [departments, setDepartments] = useState<Row[]>([]);
  const [offices, setOffices] = useState<Row[]>([]);
  const [evidence, setEvidence] = useState<Row[]>([]);
  const [history, setHistory] = useState<Row[]>([]);
  const [hierarchy, setHierarchy] = useState<Row[]>([]);
  const [posts, setPosts] = useState<Row[]>([]);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [entityType, setEntityType] = useState("other_public_agency");
  const [source, setSource] = useState("");
  const [sourceURL, setSourceURL] = useState("");
  const [department, setDepartment] = useState("");
  const [office, setOffice] = useState("");
  const [channel, setChannel] = useState("official_portal");
  const [endpoint, setEndpoint] = useState("");
  const [place, setPlace] = useState("");
  const [category, setCategory] = useState("");
  const [resolution, setResolution] = useState("");
  const [loaded, setLoaded] = useState(false);

  function fail(caught: unknown, fallback: string) {
    setError(caught instanceof ApiError ? caught.message : fallback);
  }

  function load() {
    if (!token) return;
    api<{ data: Authority[] }>("/api/v1/authorities", { token })
      .then((response) => { setError(""); setRows(response.data); setLoaded(true); })
      .catch((caught) => { setLoaded(false); fail(caught, "Could not load the directory."); });
    api<{ data: Row[] }>("/api/v1/posts", { token }).then((response) => setPosts(response.data)).catch(() => setPosts([]));
  }

  function open(id: string) {
    if (!token) return;
    setError("");
    api<Authority>(`/api/v1/authorities/${id}`, { token }).then(setSelected).catch((caught) => fail(caught, "Could not open that record."));
    api<{ data: Row[] }>(`/api/v1/authorities/${id}/departments`, { token }).then((response) => setDepartments(response.data)).catch(() => setDepartments([]));
    api<{ data: Row[] }>(`/api/v1/authorities/${id}/evidence`, { token }).then((response) => setEvidence(response.data)).catch(() => setEvidence([]));
    api<{ data: Row[] }>(`/api/v1/authorities/${id}/history`, { token }).then((response) => setHistory(response.data)).catch(() => setHistory([]));
    api<{ data: Row[] }>(`/api/v1/authorities/${id}/hierarchy`, { token }).then((response) => setHierarchy(response.data)).catch(() => setHierarchy([]));
  }

  useEffect(() => { load(); }, [token]);

  async function create(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    try {
      const created = await api<{ id: string }>("/api/v1/authorities", { method: "POST", token, body: { official_name: name, entity_type: entityType } });
      setName("");
      load();
      open(created.id);
    } catch (caught) {
      fail(caught, "Could not save the draft.");
    }
  }

  async function addEvidence(event: React.FormEvent) {
    event.preventDefault();
    if (!selected) return;
    setError("");
    try {
      await api(`/api/v1/authorities/${selected.id}/evidence`, { method: "POST", token, body: { method: "official_website", source_url: sourceURL, description: source } });
      open(selected.id);
      setSource("");
      setSourceURL("");
    } catch (caught) {
      fail(caught, "Could not save the evidence.");
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-serif text-3xl">Authority registry</h1>
      <p className="max-w-3xl text-sm">A draft, pending, suspended, or expired record is not ready to receive an automatic complaint. Verification needs source evidence. This page does not send a complaint, email, or message.</p>
      {error ? <ErrorText error={error} /> : null}
      {can("authority.registry.create") ? (
        <Card>
          <form className="grid gap-3 md:grid-cols-3" onSubmit={create}>
            <Field label="Official name"><input className={controlClass} required value={name} onChange={(event) => setName(event.target.value)} /></Field>
            <Field label="Authority type">
              <select className={controlClass} value={entityType} onChange={(event) => setEntityType(event.target.value)}>
                {types.map((type) => <option key={type} value={type}>{type.replaceAll("_", " ")}</option>)}
              </select>
            </Field>
            <div className="flex items-end"><Button type="submit">Save draft</Button></div>
          </form>
        </Card>
      ) : null}
      {loaded && rows.length === 0 ? <Card><p>No authority records are on file. Nothing here is a government partnership or an official contact.</p></Card> : null}
      {rows.length > 0 ? (
        <ul className="grid gap-3">
          {rows.map((row) => (
            <li key={row.id}>
              <button className="w-full border border-black bg-card p-3 text-left" onClick={() => open(row.id)}>
                <span className="flex flex-wrap items-center gap-2"><span className="font-semibold">{row.official_name}</span>{badge(row.verification_state)}</span>
                <span className="mt-1 block text-sm">{row.entity_type.replaceAll("_", " ")} · not ready for automatic routing</span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      {selected ? (
        <div className="grid gap-4">
          <Card>
            <h2 className="font-serif text-2xl">{selected.official_name}</h2>
            <p className="mt-2 text-sm">{selected.notice}</p>
            <p className="mt-2">{badge(selected.verification_state)}</p>
            {can("authority.registry.update") ? (
              <form className="mt-4 grid gap-3" onSubmit={addEvidence}>
                <Field label="Evidence source URL"><input className={controlClass} required type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} /></Field>
                <Field label="What the source shows"><input className={controlClass} required value={source} onChange={(event) => setSource(event.target.value)} /></Field>
                <Button type="submit">Attach evidence</Button>
              </form>
            ) : null}
            {can("authority.registry.verify") ? (
              <div className="mt-3">
                <Button onClick={() => api(`/api/v1/authorities/${selected.id}/state`, { method: "POST", token, body: { verification_state: "VERIFIED", note: "Reviewed against the attached source." } }).then(() => open(selected.id)).catch((caught) => fail(caught, "Verification was rejected."))}>Record verification decision</Button>
                <p className="mt-2 text-sm">The service rejects this when evidence is missing, and a high-impact record also needs a different reviewer.</p>
              </div>
            ) : null}
          </Card>
          <Card>
            <h2 className="font-serif text-xl">Evidence</h2>
            {evidence.length === 0 ? <p className="mt-2 text-sm">No evidence is attached.</p> : evidence.map((item) => <p key={item.id} className="mt-2 text-sm">{item.method} · {item.source_url} · {item.description}</p>)}
          </Card>
          <Card>
            <h2 className="font-serif text-xl">Departments and offices</h2>
            {can("authority.department.manage") ? (
              <form className="mt-3 flex flex-col gap-3 md:flex-row" onSubmit={(event) => { event.preventDefault(); api(`/api/v1/authorities/${selected.id}/departments`, { method: "POST", token, body: { official_name: department, functional_responsibility: department } }).then(() => { setDepartment(""); open(selected.id); }).catch((caught) => fail(caught, "Could not add the department.")); }}>
                <input className={controlClass} required placeholder="Department name" value={department} onChange={(event) => setDepartment(event.target.value)} />
                <Button type="submit">Add department</Button>
              </form>
            ) : null}
            {departments.map((item) => (
              <div key={item.id} className="mt-3 border border-black p-3">
                <p className="font-semibold">{item.official_name}</p>
                <button className="mt-2 text-sm underline" onClick={() => api<{ data: Row[] }>(`/api/v1/departments/${item.id}/offices`, { token }).then((response) => setOffices(response.data))}>Show offices</button>
              </div>
            ))}
            {can("authority.office.manage") && departments[0] ? (
              <form className="mt-3 flex flex-col gap-3 md:flex-row" onSubmit={(event) => { event.preventDefault(); api(`/api/v1/departments/${departments[0].id}/offices`, { method: "POST", token, body: { official_name: office, office_type: "field_office" } }).then(() => { setOffice(""); api<{ data: Row[] }>(`/api/v1/departments/${departments[0].id}/offices`, { token }).then((response) => setOffices(response.data)); }).catch((caught) => fail(caught, "Could not add the office.")); }}>
                <input className={controlClass} required placeholder="Office name" value={office} onChange={(event) => setOffice(event.target.value)} />
                <Button type="submit">Add office under the first department</Button>
              </form>
            ) : null}
            {offices.map((item) => <p key={item.id} className="mt-2 text-sm">{item.official_name} · {badge(item.verification_state || "DRAFT")}</p>)}
          </Card>
          <Card>
            <h2 className="font-serif text-xl">Official posts</h2>
            <p className="mt-2 text-sm">A post is separate from the person who occupies it. This screen does not create a government account.</p>
            <ul className="mt-2 grid gap-1 text-sm">{posts.map((post) => <li key={post.id}>{post.name}</li>)}</ul>
          </Card>
          <Card>
            <h2 className="font-serif text-xl">Contact channel</h2>
            <p className="mt-2 text-sm">A new channel stays unverified. A personal address is not an official channel.</p>
            {can("authority.contact.manage") ? (
              <form className="mt-3 grid gap-3" onSubmit={(event) => { event.preventDefault(); api(`/api/v1/authorities/${selected.id}/contacts`, { method: "POST", token, body: { channel_type: channel, endpoint, visibility: "internal" } }).then(() => setEndpoint("")).catch((caught) => fail(caught, "Could not save the channel.")); }}>
                <select className={controlClass} value={channel} onChange={(event) => setChannel(event.target.value)}>
                  {["official_email", "office_telephone", "official_portal", "government_api", "whatsapp_business", "physical_handoff"].map((item) => <option key={item} value={item}>{item.replaceAll("_", " ")}</option>)}
                </select>
                <input className={controlClass} required placeholder="Endpoint from a source you can cite" value={endpoint} onChange={(event) => setEndpoint(event.target.value)} />
                <Button type="submit">Save unverified channel</Button>
              </form>
            ) : null}
          </Card>
          <Card>
            <h2 className="font-serif text-xl">Jurisdiction and responsibility</h2>
            <p className="mt-2 text-sm">A locality name is not a verified jurisdiction. No district, taluk, ward, or municipal match is invented.</p>
            {can("authority.jurisdiction.manage") ? (
              <form className="mt-3 flex flex-col gap-3 md:flex-row" onSubmit={(event) => { event.preventDefault(); api(`/api/v1/authorities/${selected.id}/jurisdictions`, { method: "POST", token, body: { level: "other", name: place, locality_text: place } }).then(() => setPlace("")).catch((caught) => fail(caught, "Could not save the jurisdiction reference.")); }}>
                <input className={controlClass} required placeholder="Unverified area name" value={place} onChange={(event) => setPlace(event.target.value)} />
                <Button type="submit">Save unverified area</Button>
              </form>
            ) : null}
            {can("authority.responsibility.manage") ? (
              <form className="mt-3 flex flex-col gap-3 md:flex-row" onSubmit={(event) => { event.preventDefault(); api(`/api/v1/authorities/${selected.id}/responsibilities`, { method: "POST", token, body: { category_key: category, responsibility: "primary", ownership_note: "Recorded for review. Ownership is not confirmed." } }).then(() => setCategory("")).catch((caught) => fail(caught, "Could not save the rule.")); }}>
                <input className={controlClass} required placeholder="Category key" value={category} onChange={(event) => setCategory(event.target.value)} />
                <Button type="submit">Save draft responsibility</Button>
              </form>
            ) : null}
            <h3 className="mt-4 font-semibold">Stored relationships</h3>
            {hierarchy.length === 0 ? <p className="mt-2 text-sm">No supervisory relationship is stored.</p> : hierarchy.map((item) => <p key={item.id} className="mt-2 text-sm">{item.relation.replaceAll("_", " ")}</p>)}
          </Card>
          <Card>
            <h2 className="font-serif text-xl">Resolution check</h2>
            <form className="mt-3 grid gap-3" onSubmit={(event) => { event.preventDefault(); api("/api/v1/authority-resolution", { method: "POST", token, body: { category_key: category, locality_text: place, provenance: "HUMAN_VERIFIED" } }).then((body) => setResolution(JSON.stringify(body, null, 2))).catch((caught) => fail(caught, "Resolution failed.")); }}>
              <Button type="submit">Check responsibility</Button>
            </form>
            {resolution ? <pre className="mt-3 overflow-auto text-xs">{resolution}</pre> : null}
          </Card>
          <Card>
            <h2 className="font-serif text-xl">Audit</h2>
            {history.length === 0 ? <p className="mt-2 text-sm">No audit rows are visible for this record.</p> : history.map((item, index) => <p key={`${item.action}-${index}`} className="mt-2 text-sm">{item.action} · {item.detail}</p>)}
          </Card>
        </div>
      ) : null}
    </div>
  );
}
