"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Button, Card, ErrorText, Field, controlClass, fieldErrors } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Created = {
  issue: { id: string };
  safety?: { message: string; channels: string[]; dispatched: boolean };
};

type TaxonomyType = { id: string; name: string; prompt: string | null; fields?: { key: string; label: string }[] };
type TaxonomyCategory = { id: string; name: string; problem_types: TaxonomyType[] };

export default function NewReportPage() {
  const { token } = useAuth();
  const router = useRouter();
  const [description, setDescription] = useState("");
  const [types, setTypes] = useState<Array<TaxonomyType & { label: string }>>([]);
  const [problemTypeId, setProblemTypeId] = useState("");
  const [category, setCategory] = useState("");
  const [urgency, setUrgency] = useState("normal");
  const [address, setAddress] = useState("");
  const [locality, setLocality] = useState("");
  const [latitude, setLatitude] = useState("");
  const [longitude, setLongitude] = useState("");
  const [visible, setVisible] = useState(false);
  const [followPublic, setFollowPublic] = useState(false);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [error, setError] = useState<ApiError | null>(null);
  const [pending, setPending] = useState(false);
  const fields = fieldErrors(error);
  const selected = types.find((item) => item.id === problemTypeId);

  useEffect(() => {
    if (!token) return;
    api<{ data: TaxonomyCategory[] }>("/api/v1/taxonomy", { token })
      .then((response) => {
        setTypes(
          response.data.flatMap((item) =>
            item.problem_types.map((problem) => ({ ...problem, label: `${item.name} · ${problem.name}` })),
          ),
        );
      })
      .catch(() => setTypes([]));
  }, [token]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setPending(true);
    setError(null);
    const lat = latitude.trim() === "" ? null : Number(latitude);
    const lng = longitude.trim() === "" ? null : Number(longitude);
    try {
      const created = await api<Created>("/api/v1/issues", {
        method: "POST",
        token,
        body: {
          description,
          problem_type_id: problemTypeId || null,
          category_label: category || null,
          stated_urgency: urgency,
          address_text: address || null,
          locality_text: locality || null,
          latitude: lat,
          longitude: lng,
          identity_visible_to_resolver: visible,
          follow_public: followPublic,
          answers,
        },
      });
      if (created.safety) sessionStorage.setItem(`lokasolve-safety-${created.issue.id}`, created.safety.message);
      router.push(`/reports/${created.issue.id}`);
    } catch (caught) {
      setError(caught instanceof ApiError ? caught : new ApiError(0, "network", "Could not save the report."));
    } finally {
      setPending(false);
    }
  }

  return (
    <form className="desk" onSubmit={submit}>
      <header>
        <h1>Tell us what is wrong</h1>
      </header>
      {error && Object.keys(fields).length === 0 ? <ErrorText error={error.message} /> : null}
      <Field label="What happened" error={fields.description}>
        <textarea className={`${controlClass} min-h-32 py-3`} required minLength={8} value={description} onChange={(event) => setDescription(event.target.value)} />
      </Field>
      {types.length > 0 ? (
        <Field label="Problem type">
          <select className={controlClass} value={problemTypeId} onChange={(event) => setProblemTypeId(event.target.value)}>
            <option value="">I will describe it</option>
            {types.map((item) => (
              <option key={item.id} value={item.id}>
                {item.label}
              </option>
            ))}
          </select>
          {selected?.prompt ? <p className="mt-2 text-sm">{selected.prompt}</p> : null}
        </Field>
      ) : null}
      {selected?.fields?.map((field) => (
        <Field key={field.key} label={field.label}>
          <input className={controlClass} value={answers[field.key] ?? ""} onChange={(event) => setAnswers((current) => ({ ...current, [field.key]: event.target.value }))} />
        </Field>
      ))}
      <Field label="Label, if you know one" error={fields.category_label}>
        <input className={controlClass} value={category} onChange={(event) => setCategory(event.target.value)} placeholder="Street light, water, drain, or anything else" />
      </Field>
      <Field label="How urgent it feels" error={fields.stated_urgency}>
        <select className={controlClass} value={urgency} onChange={(event) => setUrgency(event.target.value)}>
          <option value="low">Low</option>
          <option value="normal">Normal</option>
          <option value="high">High</option>
          <option value="immediate_risk">Immediate danger</option>
        </select>
      </Field>
      <Field label="Locality" error={fields.locality_text}>
        <input className={controlClass} value={locality} onChange={(event) => setLocality(event.target.value)} />
      </Field>
      <Field label="Address or landmark" error={fields.address_text}>
        <input className={controlClass} value={address} onChange={(event) => setAddress(event.target.value)} />
      </Field>
      <div className="flex flex-wrap gap-3">
        <button
          type="button"
          className="btn btn-line"
          onClick={() => {
            if (!navigator.geolocation) return;
            navigator.geolocation.getCurrentPosition((position) => {
              setLatitude(String(position.coords.latitude));
              setLongitude(String(position.coords.longitude));
            });
          }}
        >
          Use this device's location
        </button>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Latitude, optional" error={fields.latitude}>
          <input className={controlClass} inputMode="decimal" value={latitude} onChange={(event) => setLatitude(event.target.value)} />
        </Field>
        <Field label="Longitude, optional" error={fields.longitude}>
          <input className={controlClass} inputMode="decimal" value={longitude} onChange={(event) => setLongitude(event.target.value)} />
        </Field>
      </div>
      <label className="flex items-start gap-2 text-sm">
        <input type="checkbox" checked={visible} onChange={(event) => setVisible(event.target.checked)} />
        <span>If someone is assigned later, my name may be shown to them.</span>
      </label>
      <label className="flex items-start gap-2 text-sm">
        <input type="checkbox" checked={followPublic} onChange={(event) => setFollowPublic(event.target.checked)} />
        <span>Show the place and description on Nearby. My name stays off that list.</span>
      </label>
      {urgency === "immediate_risk" ? (
        <Card>
          <p className="font-semibold">This is not emergency dispatch.</p>
          <p className="mt-2 text-sm">If someone is in immediate danger, contact 112. For a mental-health crisis, Tele-MANAS is 14416. Do not wait for LokaSolve.</p>
        </Card>
      ) : null}
      <div className="flex flex-wrap gap-3">
        <Button type="submit" disabled={pending}>
          {pending ? "Saving…" : "Save report"}
        </Button>
        <Link href="/reports" className="btn btn-line">
          Cancel
        </Link>
      </div>
    </form>
  );
}
