export type BuildState = "live" | "partial" | "not_operating";

export type PlatformSection = {
  slug: string;
  group: string;
  title: string;
  state: BuildState;
  summary: string;
  points: string[];
};

export const platformSections: PlatformSection[] = [
  {
    slug: "account",
    group: "Resident",
    title: "Account and profile",
    state: "partial",
    summary: "A person can create an account, sign in, set a language, save a place, and request account deletion.",
    points: [
      "Email and password sign-in is operating.",
      "Saved places store an address as text. Coordinates are stored only when the person supplies them.",
      "Email verification, guest reports, and WhatsApp linking are not operating.",
    ],
  },
  {
    slug: "report",
    group: "Resident",
    title: "Report any local problem",
    state: "partial",
    summary: "A report starts from the person's own words. The category is optional and is not a fixed department list.",
    points: [
      "Free text, an optional label, urgency, and an address or locality can be saved.",
      "Coordinates are saved only when both numbers are entered. They are never invented from an address.",
      "Photos, video, voice notes, and a map pin are not operating yet.",
    ],
  },
  {
    slug: "my-reports",
    group: "Resident",
    title: "My reports",
    state: "partial",
    summary: "Each person sees only the reports they created. A report is not a master incident and not an assignment.",
    points: [
      "The list and the detail page read the signed-in person's reports.",
      "Filters for assigned, disputed, and reopened stay empty because those states are not written yet.",
      "Another person's private report is not returned.",
    ],
  },
  {
    slug: "nearby",
    group: "Resident",
    title: "Nearby",
    state: "not_operating",
    summary: "A public locality feed is planned. It is not on.",
    points: [
      "No map, no distance, and no public incident list are shown.",
      "Reporter names, phone numbers, private messages, and exact home locations will stay out of any public feed.",
    ],
  },
  {
    slug: "search",
    group: "Resident",
    title: "Search",
    state: "not_operating",
    summary: "Search across issues, incidents, services, and localities is not built.",
    points: ["Results would have to follow the same visibility rules as the rest of the product."],
  },
  {
    slug: "taxonomy",
    group: "Configuration",
    title: "Taxonomy and workflows",
    state: "partial",
    summary: "Categories and problem types are loaded from the taxonomy service. A report can still be saved with only a description.",
    points: [
      "An administrator with taxonomy access can add a problem type and archive a category.",
      "The report form does not lock the person to one department.",
      "Workflows and skills are not configured from this screen.",
    ],
  },
  {
    slug: "understanding",
    group: "Understanding",
    title: "Assisted understanding",
    state: "not_operating",
    summary: "A separate service can read a description in tests. It is not connected to a saved report.",
    points: [
      "Nothing on a report is labeled as an official classification.",
      "A model guess is not stored as a fact.",
      "Missing location is asked for. It is not filled in.",
    ],
  },
  {
    slug: "incidents",
    group: "Understanding",
    title: "Incidents and duplicates",
    state: "not_operating",
    summary: "A person's report stays separate from a master incident. Linking similar reports is not operating.",
    points: [
      "Similar words in two different places will not be merged, because merging is not running at all.",
      "There is no score, no human review queue, and no merge or split action.",
    ],
  },
  {
    slug: "geography",
    group: "Understanding",
    title: "Places and boundaries",
    state: "partial",
    summary: "The database can store geography. Jurisdiction polygons and nearby search are not in use.",
    points: [
      "A saved place or a report keeps the original address text.",
      "No boundary dataset is assumed to be complete.",
    ],
  },
  {
    slug: "resolvers",
    group: "Resolution",
    title: "Resolvers and routing",
    state: "not_operating",
    summary: "No person, business, or office is offered a case from this build.",
    points: [
      "Eligibility, coverage, skills, and workload are not evaluated.",
      "The product does not say a resolver has received or accepted anything.",
    ],
  },
  {
    slug: "field",
    group: "Resolution",
    title: "Field work",
    state: "not_operating",
    summary: "Offers, quotes, schedules, arrival, and completion evidence are not operating.",
    points: [
      "A provider cannot mark a case resolved.",
      "Exact location is not shared, because no assignment exists.",
    ],
  },
  {
    slug: "digital",
    group: "Resolution",
    title: "Digital handoff",
    state: "not_operating",
    summary: "Drafts, portal links, and external reference numbers are not sent.",
    points: [
      "A link that is only a referral would not be shown as a submitted complaint.",
      "No government or utility API is connected.",
    ],
  },
  {
    slug: "lifecycle",
    group: "Resolution",
    title: "Status and escalation",
    state: "partial",
    summary: "A new report is stored as submitted. Later states are reserved and are not written by the screen.",
    points: [
      "The client cannot set a status.",
      "Escalation timers are not running.",
    ],
  },
  {
    slug: "messages",
    group: "Communication",
    title: "Notifications",
    state: "not_operating",
    summary: "In-app, email, SMS, and WhatsApp delivery are not sent from this build.",
    points: [
      "Opt-in is stored as a preference only.",
      "No message is marked delivered.",
    ],
  },
  {
    slug: "confirmation",
    group: "Communication",
    title: "Confirmation and disputes",
    state: "not_operating",
    summary: "A resolution is not confirmed, disputed, or reopened yet.",
    points: ["A provider statement alone would not close a case once this exists."],
  },
  {
    slug: "operations",
    group: "Operations",
    title: "Operations desk",
    state: "partial",
    summary: "People with operations access can see accounts. The wider console is not built.",
    points: [
      "Taxonomy editing, merge tools, routing rules, and audit export are not on the screen.",
      "Ordinary accounts do not see the operations list.",
    ],
  },
  {
    slug: "measurement",
    group: "Operations",
    title: "Outcome measurement",
    state: "not_operating",
    summary: "Resolution time, acceptance, and satisfaction are not measured yet.",
    points: ["No chart claims a faster resolution. There is no baseline."],
  },
  {
    slug: "safety",
    group: "Safety",
    title: "Immediate danger",
    state: "partial",
    summary: "If a report is marked as immediate risk, the screen shows official numbers and says nobody was dispatched.",
    points: [
      "India emergency response: 112.",
      "Tele-MANAS mental-health support: 14416.",
      "The report is still only submitted. It is not sent to a responder.",
    ],
  },
];

export function platformSection(slug: string) {
  return platformSections.find((section) => section.slug === slug);
}

export const stateLabel: Record<BuildState, string> = {
  live: "Live",
  partial: "Partial",
  not_operating: "Not operating",
};
