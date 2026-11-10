"use client";

import { useEffect, useState } from "react";
import { Button, Card, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Office = { id: string; official_name: string; org_type: string; locality_text: string | null; categories: string; verification_status: string; verification_source: string | null };

export default function AuthoritiesPage() {
  const { token } = useAuth();
  const [offices, setOffices] = useState<Office[]>([]);
  const [name, setName] = useState("");
  const [place, setPlace] = useState("");
  const [categories, setCategories] = useState("");
  const [source, setSource] = useState("");
  const [error, setError] = useState("");

  function load() {
    if (!token) return;
    api<{ data: Office[] }>("/api/v1/admin/authorities", { token }).then((response) => setOffices(response.data)).catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load offices."));
  }

  useEffect(() => { load(); }, [token]);

  async function add(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await api("/api/v1/admin/authorities", { method: "POST", token, body: { official_name: name, org_type: "department", locality_text: place, categories, verification_source: source } });
      setName("");
      setCategories("");
      load();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not save the office.");
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-serif text-3xl">Authorities</h1>
      {error ? <ErrorText error={error} /> : null}
      <Card>
        <form className="flex flex-col gap-3" onSubmit={add}>
          <Field label="Official name"><input className={controlClass} required value={name} onChange={(event) => setName(event.target.value)} /></Field>
          <Field label="Locality"><input className={controlClass} value={place} onChange={(event) => setPlace(event.target.value)} /></Field>
          <Field label="Categories"><input className={controlClass} value={categories} onChange={(event) => setCategories(event.target.value)} placeholder="Lighting, Roads" /></Field>
          <Field label="Where this record came from"><input className={controlClass} required value={source} onChange={(event) => setSource(event.target.value)} /></Field>
          <Button type="submit">Add office</Button>
        </form>
      </Card>
      <ul className="grid gap-3">
        {offices.map((office) => (
          <li key={office.id} className="border border-black bg-card p-3">
            <p className="font-semibold">{office.official_name}</p>
            <p className="mt-1 text-sm">{office.locality_text || "No locality"} · {office.categories || "No categories"} · {office.verification_status}</p>
            <p className="mt-1 text-sm">{office.verification_source}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              <Button onClick={() => api(`/api/v1/admin/authorities/${office.id}/verification`, { method: "POST", token, body: { verification_status: "verified" } }).then(load)}>Mark verified</Button>
              <Button onClick={() => api(`/api/v1/admin/authorities/${office.id}/channels`, { method: "POST", token, body: { channel: "manual", purpose: "Operator handoff" } }).then(load)}>Add manual channel</Button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
