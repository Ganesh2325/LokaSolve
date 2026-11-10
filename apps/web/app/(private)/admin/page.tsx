"use client";

import { useEffect, useState } from "react";
import { Button, Card, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type ListedUser = { id: string; email: string | null; display_name: string; status: string; is_demo: boolean };
type Analytics = { reports: number; open_reviews: number; awaiting_acceptance: number; awaiting_confirmation: number; resolved: number; older_than_48_hours: number; public_nearby: number; external_messages_ready: number };
type Review = { id: string; issue_id: string; reason: string };

export default function OperationsPage() {
  const { token, user } = useAuth();
  const [users, setUsers] = useState<ListedUser[]>([]);
  const [error, setError] = useState("");
  const [categoryName, setCategoryName] = useState("");
  const [typeName, setTypeName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [fieldLabel, setFieldLabel] = useState("");
  const [resolverName, setResolverName] = useState("");
  const [resolverSkills, setResolverSkills] = useState("");
  const [resolverPlace, setResolverPlace] = useState("");
  const [resolverEmail, setResolverEmail] = useState("");
  const [stats, setStats] = useState<Analytics | null>(null);
  const [reviews, setReviews] = useState<Review[]>([]);
  const [saved, setSaved] = useState("");
  const [taxonomyError, setTaxonomyError] = useState("");
  const canManageTypes = user?.roles.includes("super_admin") ?? false;

  useEffect(() => {
    if (!token) return;
    api<{ data: ListedUser[] }>("/api/v1/admin/users", { token })
      .then((response) => setUsers(response.data))
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load accounts."));
    api<Analytics>("/api/v1/admin/analytics", { token }).then(setStats).catch(() => setStats(null));
    api<{ data: Review[] }>("/api/v1/admin/reviews", { token }).then((response) => setReviews(response.data)).catch(() => setReviews([]));
  }, [token]);

  async function addType(event: React.FormEvent) {
    event.preventDefault();
    setSaved("");
    setTaxonomyError("");
    try {
      await api("/api/v1/admin/problem-types", {
        method: "POST",
        token,
        body: { category_name: categoryName, name: typeName, prompt, field_label: fieldLabel },
      });
      setSaved(`${categoryName} · ${typeName} is available on the report form.`);
      setTypeName("");
      setPrompt("");
    } catch (caught) {
      setTaxonomyError(caught instanceof ApiError ? caught.message : "Could not save the problem type.");
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-serif text-4xl">Operations</h1>
      {canManageTypes ? (
        <Card>
          <h2 className="font-serif text-2xl">Problem types</h2>
          <form className="mt-3 flex flex-col gap-3" onSubmit={addType}>
            <Field label="Category">
              <input className={controlClass} required minLength={2} value={categoryName} onChange={(event) => setCategoryName(event.target.value)} />
            </Field>
            <Field label="Type">
              <input className={controlClass} required minLength={2} value={typeName} onChange={(event) => setTypeName(event.target.value)} />
            </Field>
            <Field label="Prompt shown on the form">
              <input className={controlClass} value={prompt} onChange={(event) => setPrompt(event.target.value)} />
            </Field>
            <Field label="Extra question on the form">
              <input className={controlClass} value={fieldLabel} onChange={(event) => setFieldLabel(event.target.value)} />
            </Field>
            {taxonomyError ? <ErrorText error={taxonomyError} /> : null}
            {saved ? <p className="text-sm font-semibold">{saved}</p> : null}
            <Button type="submit">Add type</Button>
          </form>
        </Card>
      ) : null}
      {stats ? (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          {[
            ["Reports", stats.reports],
            ["Open reviews", stats.open_reviews],
            ["Waiting to accept", stats.awaiting_acceptance],
            ["Waiting to confirm", stats.awaiting_confirmation],
            ["Resolved", stats.resolved],
            ["Open over 48 hours", stats.older_than_48_hours],
            ["Public nearby", stats.public_nearby],
            ["External messages sent", stats.external_messages_ready],
          ].map(([label, value]) => (
            <Card key={label}>
              <p className="text-xs font-bold uppercase">{label}</p>
              <p className="font-serif text-3xl">{value}</p>
            </Card>
          ))}
        </div>
      ) : null}
      {canManageTypes ? (
        <Card>
          <h2 className="font-serif text-2xl">Responders</h2>
          <form className="mt-3 flex flex-col gap-3" onSubmit={async (event) => {
            event.preventDefault();
            setTaxonomyError("");
            try {
              await api("/api/v1/admin/resolvers", { method: "POST", token, body: { display_name: resolverName, kind: "provider", skills: resolverSkills, locality_text: resolverPlace, owner_email: resolverEmail } });
              setSaved(`${resolverName} is on the directory.`);
              setResolverName("");
            } catch (caught) {
              setTaxonomyError(caught instanceof ApiError ? caught.message : "Could not save the responder.");
            }
          }}>
            <Field label="Name"><input className={controlClass} required value={resolverName} onChange={(event) => setResolverName(event.target.value)} /></Field>
            <Field label="Skills"><input className={controlClass} required value={resolverSkills} onChange={(event) => setResolverSkills(event.target.value)} placeholder="Lighting, Roads" /></Field>
            <Field label="Locality"><input className={controlClass} value={resolverPlace} onChange={(event) => setResolverPlace(event.target.value)} /></Field>
            <Field label="Account email, optional"><input className={controlClass} value={resolverEmail} onChange={(event) => setResolverEmail(event.target.value)} /></Field>
            <Button type="submit">Add responder</Button>
          </form>
        </Card>
      ) : null}
      {reviews.length > 0 ? (
        <Card>
          <h2 className="font-serif text-2xl">Review queue</h2>
          <ul className="mt-3 flex flex-col gap-3">
            {reviews.map((item) => (
              <li key={item.id} className="text-sm">
                <p>{item.reason}</p>
                <Button className="mt-2" onClick={() => api(`/api/v1/admin/reviews/${item.id}/resolve`, { method: "POST", token }).then(() => setReviews((current) => current.filter((review) => review.id !== item.id)))}>Reviewed</Button>
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
      {error ? <ErrorText error={error} /> : null}
      {!error ? (
        <Card>
          <h2 className="font-serif text-2xl">Accounts</h2>
          <ul className="mt-3 flex flex-col gap-2">
            {users.map((user) => (
              <li key={user.id} className="text-sm">
                {user.display_name} · {user.email} · {user.status}
                {user.is_demo ? " · demo" : ""}
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
    </div>
  );
}
