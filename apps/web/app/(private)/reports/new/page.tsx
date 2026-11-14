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

type FieldDef = { key: string; label: string; type?: string; required?: boolean; max_length?: number; pattern?: string };
type Evidence = { kind: string; label: string; required: boolean };
type TaxonomyType = { id: string; name: string; prompt: string | null; fields?: FieldDef[]; evidence?: Evidence[]; schema_version?: number };
type TaxonomyCategory = { id: string; name: string; problem_types: TaxonomyType[] };

export default function NewReportPage() {
  const { token } = useAuth();
  const router = useRouter();
  const [description, setDescription] = useState("");
  const [types, setTypes] = useState<Array<TaxonomyType & { label: string; category: string }>>([]);
  const [search, setSearch] = useState("");
  const [taxonomyNote, setTaxonomyNote] = useState("");
  const [taxonomySource, setTaxonomySource] = useState<"live" | "cached" | "none">("none");
  const [problemTypeId, setProblemTypeId] = useState("");
  const [category, setCategory] = useState("");
  const [urgency, setUrgency] = useState("normal");
  const [address, setAddress] = useState("");
  const [locality, setLocality] = useState("");
  const [latitude, setLatitude] = useState("");
  const [longitude, setLongitude] = useState("");
  const [placeNote, setPlaceNote] = useState("A written place is enough to save this report. A person reviews the place. Coordinates, when you add them, are the location you reported.");
  const [visible, setVisible] = useState(false);
  const [followPublic, setFollowPublic] = useState(false);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [error, setError] = useState<ApiError | null>(null);
  const [pending, setPending] = useState(false);
  const fields = fieldErrors(error);

  useEffect(() => {
    const lat = Number(latitude);
    const lng = Number(longitude);
    const fallback = "A written place is enough to save this report. A person reviews the place. Coordinates, when you add them, are the location you reported.";
    if (!token || latitude.trim() === "" || longitude.trim() === "" || !Number.isFinite(lat) || !Number.isFinite(lng)) {
      setPlaceNote(fallback);
      return;
    }
    let cancelled = false;
    api<{ human_review_required?: boolean; matches?: Array<{ name?: string; verification_status?: string; level?: string }> }>("/api/v1/geospatial/resolve", {
      method: "POST",
      token,
      body: { latitude: lat, longitude: lng },
    })
      .then((result) => {
        if (cancelled) return;
        const verified = (result.matches ?? []).filter((match) => match.verification_status === "verified");
        if (verified.length === 0) {
          setPlaceNote("No verified boundary contains this point. Saving does not assign a ward, district, or office. A person reviews the place you wrote.");
          return;
        }
        const names = verified.map((match) => match.name).filter(Boolean).slice(0, 3).join(", ");
        setPlaceNote(`${names} ${verified.length === 1 ? "is a stored boundary" : "are stored boundaries"}. This lookup does not assign a government office.`);
      })
      .catch(() => {
        if (!cancelled) setPlaceNote(fallback);
      });
    return () => {
      cancelled = true;
    };
  }, [latitude, longitude, token]);
  const selected = types.find((item) => item.id === problemTypeId);
  const query = search.trim().toLowerCase();
  const visibleTypes = types.filter((item) => query === "" || item.label.toLowerCase().includes(query) || item.category.toLowerCase().includes(query));

  function applyTypes(categories: TaxonomyCategory[]) {
    setTypes(
      categories.flatMap((item) =>
        item.problem_types.map((problem) => ({ ...problem, category: item.name, label: `${item.name} · ${problem.name}` })),
      ),
    );
  }

  function loadTaxonomy() {
    if (!token) return;
    const locale = typeof navigator === "undefined" ? "en" : navigator.language.slice(0, 2);
    api<{ data: TaxonomyCategory[] }>(`/api/v1/taxonomy?locale=${encodeURIComponent(locale)}`, { token })
      .then((response) => {
        applyTypes(response.data);
        setTaxonomySource("live");
        setTaxonomyNote("");
        sessionStorage.setItem("lokasolve-taxonomy-published", JSON.stringify({ saved_at: new Date().toISOString(), categories: response.data }));
      })
      .catch(() => {
        const raw = sessionStorage.getItem("lokasolve-taxonomy-published");
        if (raw) {
          try {
            const saved = JSON.parse(raw) as { saved_at?: string; categories?: TaxonomyCategory[] };
            const savedAt = saved.saved_at ? Date.parse(saved.saved_at) : 0;
            const fresh = savedAt > 0 && Date.now() - savedAt < 30 * 60 * 1000;
            if (fresh && Array.isArray(saved.categories)) {
              applyTypes(saved.categories);
              setTaxonomySource("cached");
              setTaxonomyNote(`Categories could not be loaded. Showing the list saved on this device at ${new Date(savedAt).toLocaleString()}. That list has not been checked again. You can still describe the problem and save the report.`);
              return;
            }
          } catch {
            sessionStorage.removeItem("lokasolve-taxonomy-published");
          }
        }
        if (types.length > 0) {
          setTaxonomySource("cached");
          setTaxonomyNote("Categories could not be loaded. The list already on this page has not been checked again. You can still describe the problem and save the report.");
          return;
        }
        setTaxonomySource("none");
        setTaxonomyNote("Categories could not be loaded. Describe the problem in your own words and save the report. No category has been confirmed.");
      });
  }

  useEffect(() => {
    loadTaxonomy();
    // The form fields stay as entered. Only the category list is loaded again.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setPending(true);
    setError(null);
    if (selected) {
      const missing = (selected.fields ?? []).find((field) => field.required && (answers[field.key] ?? "").trim() === "");
      if (missing) {
        setError(new ApiError(400, "validation_error", `${missing.label} is required for this problem type.`));
        setPending(false);
        return;
      }
    }
    const lat = latitude.trim() === "" ? null : Number(latitude);
    const lng = longitude.trim() === "" ? null : Number(longitude);
    try {
      const created = await api<Created>("/api/v1/issues", {
        method: "POST",
        token,
        body: {
          description,
          problem_type_id: problemTypeId || null,
          category_label: (taxonomySource === "live" ? category || selected?.category : category) || null,
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
      {taxonomyNote ? (
        <Card>
          <p className="text-sm">{taxonomyNote}</p>
          <button type="button" className="btn btn-line mt-3" onClick={loadTaxonomy}>
            Try loading categories again
          </button>
        </Card>
      ) : null}
      {taxonomySource === "cached" && selected ? (
        <p className="text-sm">This problem type is from the saved list. Saving the report does not confirm it.</p>
      ) : null}
      <Field label="Search categories and problem types">
        <input className={controlClass} value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Light, water, drain, or leave this blank" />
      </Field>
      <Field label="Problem type">
        <select className={controlClass} value={problemTypeId} onChange={(event) => setProblemTypeId(event.target.value)}>
          <option value="">I will describe it</option>
          {visibleTypes.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </select>
        {selected?.prompt ? <p className="mt-2 text-sm">{selected.prompt}</p> : null}
      </Field>
      {selected?.evidence && selected.evidence.length > 0 ? (
        <Card>
          <p className="font-semibold">What helps with this problem</p>
          <ul className="mt-2 flex flex-col gap-1 text-sm">
            {selected.evidence.map((item) => (
              <li key={`${item.kind}-${item.label}`}>{item.label}{item.required ? " Required before a resolver can use it." : ""}</li>
            ))}
          </ul>
        </Card>
      ) : null}
      {selected?.fields?.map((field) => (
        <Field key={field.key} label={field.required ? `${field.label} (required)` : field.label}>
          <input
            className={controlClass}
            required={field.required}
            maxLength={field.max_length || undefined}
            inputMode={field.type === "number" ? "decimal" : undefined}
            value={answers[field.key] ?? ""}
            onChange={(event) => setAnswers((current) => ({ ...current, [field.key]: event.target.value }))}
          />
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
      <p className="text-sm text-ink/80">{placeNote}</p>
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
