import Link from "next/link";
import { notFound } from "next/navigation";
import { platformSection, stateLabel } from "@/lib/platform";

export default async function GuideSectionPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const section = platformSection(slug);
  if (!section) notFound();

  return (
    <article className="desk">
      <header>
        <p className="kicker">{section.group}</p>
        <h1>{section.title}</h1>
        <p className="desk-lede">{section.summary}</p>
        <p className="mt-3 text-sm font-bold uppercase tracking-[0.12em]">{stateLabel[section.state]}</p>
      </header>
      <ul className="grid gap-3">
        {section.points.map((point) => (
          <li key={point} className="border border-black bg-card px-4 py-3 text-sm leading-6">
            {point}
          </li>
        ))}
      </ul>
      <p>
        <Link href="/guide" className="font-semibold underline">
          All sections
        </Link>
      </p>
    </article>
  );
}
