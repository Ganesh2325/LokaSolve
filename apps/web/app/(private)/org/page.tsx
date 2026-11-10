"use client";

import { useState } from "react";
import { Button, Card, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Member = { user_id: string; email: string; display_name: string; member_role: string };

export default function OrganizationPage() {
  const { token, organizations, refreshUser } = useAuth();
  const [name, setName] = useState("");
  const [orgType, setOrgType] = useState("municipality");
  const [email, setEmail] = useState("");
  const [members, setMembers] = useState<Member[]>([]);
  const [error, setError] = useState("");
  const selected = organizations[0];

  async function createOrg(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await api("/api/v1/organizations", { method: "POST", token, body: { name, org_type: orgType } });
      await refreshUser();
      setName("");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not create the organization.");
    }
  }

  async function loadMembers() {
    if (!selected) return;
    const response = await api<{ data: Member[] }>(`/api/v1/organizations/${selected.id}/members`, { token });
    setMembers(response.data);
  }

  async function addMember(event: React.FormEvent) {
    event.preventDefault();
    if (!selected) return;
    setError("");
    try {
      await api(`/api/v1/organizations/${selected.id}/members`, {
        method: "POST",
        token,
        body: { email, member_role: "member" },
      });
      setEmail("");
      await loadMembers();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not add that member.");
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-serif text-4xl">Organization</h1>
      {error ? <ErrorText error={error} /> : null}
      {organizations.length === 0 ? (
        <Card>
          <h2 className="font-serif text-2xl">Create an organization</h2>
          <form className="mt-4 flex flex-col gap-3" onSubmit={createOrg}>
            <Field label="Name"><input className={controlClass} required value={name} onChange={(event) => setName(event.target.value)} /></Field>
            <Field label="Type">
              <select className={controlClass} value={orgType} onChange={(event) => setOrgType(event.target.value)}>
                <option value="municipality">Municipality</option>
                <option value="utility">Utility</option>
                <option value="business">Business</option>
                <option value="ngo">NGO</option>
                <option value="institution">Institution</option>
                <option value="other">Other</option>
              </select>
            </Field>
            <Button type="submit">Create</Button>
          </form>
        </Card>
      ) : (
        <Card>
          <h2 className="font-serif text-2xl">{selected.name}</h2>
          <p className="mt-1 text-sm text-muted">{selected.org_type} · your role: {selected.member_role}{selected.is_demo ? " · demo" : ""}</p>
          <form className="mt-4 flex flex-col gap-3" onSubmit={addMember}>
            <Field label="Add a member by email">
              <input className={controlClass} type="email" required value={email} onChange={(event) => setEmail(event.target.value)} />
            </Field>
            <Button type="submit">Add member</Button>
          </form>
          <button type="button" className="mt-4 min-h-11 text-sm font-semibold text-teal" onClick={loadMembers}>Refresh members</button>
          <ul className="mt-3 flex flex-col gap-2">
            {members.map((member) => (
              <li key={member.user_id} className="text-sm">{member.display_name} · {member.email} · {member.member_role}</li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  );
}
