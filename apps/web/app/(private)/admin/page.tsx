"use client";

import { useEffect, useState } from "react";
import { Button, Card, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type ListedUser = { id: string; email: string | null; display_name: string; status: string; is_demo: boolean };
type Analytics = { reports: number; open_reviews: number; awaiting_acceptance: number; awaiting_confirmation: number; resolved: number; older_than_48_hours: number; public_nearby: number; external_messages_ready: number };
type Review = {
  id: string;
  issue_id: string;
  reason: string;
  status: string;
  severity: string | null;
  geographic_area: string | null;
  suggested_department: string | null;
  deadline_at: string | null;
};

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
  const [decisionNote, setDecisionNote] = useState("");
  const [saved, setSaved] = useState("");
  const [taxonomyError, setTaxonomyError] = useState("");
  const [categories, setCategories] = useState<Array<{ id: string; name: string; status?: string; problem_types: Array<{ id: string; name: string; schema_version?: number }> }>>([]);
  const [draftNames, setDraftNames] = useState<Record<string, string>>({});
  const canManageTypes = user?.roles.includes("super_admin") ?? false;

  useEffect(() => {
    if (!token) return;
    api<{ data: ListedUser[] }>("/api/v1/admin/users", { token })
      .then((response) => setUsers(response.data))
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load accounts."));
    api<Analytics>("/api/v1/admin/analytics", { token }).then(setStats).catch(() => setStats(null));
    api<{ data: Review[] }>("/api/v1/admin/reviews", { token }).then((response) => setReviews(response.data)).catch(() => setReviews([]));
    api<{ data: Array<{ id: string; name: string; status?: string; problem_types: Array<{ id: string; name: string }> }> }>("/api/v1/taxonomy?include=archived", { token })
      .then((response) => setCategories(response.data))
      .catch(() => setCategories([]));
  }, [token]);

  async function decide(id: string, decision: string, severity?: string) {
    setError("");
    try {
      await api(`/api/v1/admin/reviews/${id}/decision`, {
        method: "POST",
        token,
        body: { decision, note: decisionNote, suggested_department: "", severity: severity ?? "" },
      });
      setReviews((current) => current.map((review) => review.id === id ? { ...review, status: decision } : review));
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not save the review decision.");
    }
  }

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
      const listed = await api<{ data: Array<{ id: string; name: string; status?: string; problem_types: Array<{ id: string; name: string }> }> }>("/api/v1/taxonomy?include=archived", { token });
      setCategories(listed.data);
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
          {categories.length > 0 ? (
            <ul className="mt-4 flex flex-col gap-2 text-sm">
              {categories.map((item) => (
                <li key={item.id} className="flex flex-col gap-2 border-t border-ink pt-2">
                  <span>{item.name} · {item.status === "archived" ? "archived" : "active"} · {item.problem_types.map((problem) => `${problem.name}${problem.schema_version ? ` v${problem.schema_version}` : ""}`).join(", ") || "no problem types"}</span>
                  {item.status !== "archived" ? (
                    <div className="flex flex-wrap gap-2">
                      <input className={controlClass} value={draftNames[item.id] ?? item.name} aria-label={`Revised name for ${item.name}`} onChange={(event) => setDraftNames((current) => ({ ...current, [item.id]: event.target.value }))} />
                      <button
                        type="button"
                        className="btn btn-line"
                        onClick={async () => {
                          setTaxonomyError("");
                          try {
                            await api(`/api/v1/taxonomy/categories/${item.id}`, { method: "PATCH", token, body: { name: draftNames[item.id] ?? item.name, locale: "en", description: "" } });
                            const listed = await api<{ data: typeof categories }>("/api/v1/taxonomy?include=archived", { token });
                            setCategories(listed.data);
                            setSaved(`${item.name} was updated.`);
                          } catch (caught) {
                            setTaxonomyError(caught instanceof ApiError ? caught.message : "Could not update the category.");
                          }
                        }}
                      >
                        Save name
                      </button>
                    </div>
                  ) : null}
                  {item.status !== "archived" ? (
                    <button
                      type="button"
                      className="btn btn-line"
                      onClick={async () => {
                        setTaxonomyError("");
                        try {
                          await api(`/api/v1/taxonomy/categories/${item.id}/archive`, { method: "POST", token });
                          setCategories((current) => current.map((category) => category.id === item.id ? { ...category, status: "archived" } : category));
                          setSaved(`${item.name} is archived. Older reports still keep this category.`);
                        } catch (caught) {
                          setTaxonomyError(caught instanceof ApiError ? caught.message : "Could not archive the category.");
                        }
                      }}
                    >
                      Archive
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}
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
            ["External messages recorded", stats.external_messages_ready],
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
                <p className="font-semibold">{item.severity ?? "Unclassified"} · {item.status.replaceAll("_", " ")}</p>
                <p className="mt-1">{item.reason}</p>
                <p className="mt-1">{item.geographic_area ?? "No area recorded"} · {item.suggested_department ?? "No department suggested"}</p>
                <p className="mt-1">Review again {item.deadline_at ? new Date(item.deadline_at).toLocaleString() : "with no deadline"}</p>
                <input className={`${controlClass} mt-2`} value={decisionNote} onChange={(event) => setDecisionNote(event.target.value)} placeholder="Reason for the decision" aria-label="Review note" />
                <div className="mt-2 flex flex-wrap gap-2">
                  <Button onClick={() => decide(item.id, "confirm_routing")}>Confirm routing</Button>
                  <Button onClick={() => decide(item.id, "request_information")}>Ask for information</Button>
                  <Button onClick={() => decide(item.id, "escalate")}>Escalate</Button>
                  <Button onClick={() => decide(item.id, "reject_classification", "P3")}>Reject classification</Button>
                  <Button onClick={() => decide(item.id, "no_destination")}>No verified destination</Button>
                </div>
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
