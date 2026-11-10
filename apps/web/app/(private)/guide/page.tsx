import Link from "next/link";
import { platformSections, stateLabel } from "@/lib/platform";

export default function GuidePage() {
  const groups = [...new Set(platformSections.map((section) => section.group))];
  return (
    <div className="desk">
      <header>
        <p className="kicker">The whole platform</p>
        <h1>What this build actually does</h1>
        <p className="desk-lede">
          Every part of the resolution network is listed here. A section is partial when some of it is saved for real, and not operating when the screen must not pretend the work has happened.
        </p>
      </header>
      {groups.map((group) => (
        <section key={group}>
          <h2 className="font-serif text-2xl">{group}</h2>
          <ul className="mt-3 grid gap-3">
            {platformSections
              .filter((section) => section.group === group)
              .map((section) => (
                <li key={section.slug}>
                  <Link href={`/guide/${section.slug}`} className="block border border-black bg-card px-4 py-3">
                    <span className="text-xs font-bold uppercase tracking-[0.12em]">{stateLabel[section.state]}</span>
                    <h3 className="mt-1 font-semibold">{section.title}</h3>
                    <p className="mt-1 text-sm">{section.summary}</p>
                  </Link>
                </li>
              ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
