# MVP requirements — Stage 1 slice

The full product is a universal intake and resolution network. This document records what the current build satisfies and what remains a requirement rather than a feature.

## Product rules already binding the code

- The system does not invent coordinates, addresses, categories, provider availability, or official acceptance.
- AI output, when a model is added, may recommend. It may not assign work, take payment, submit an official complaint, or claim emergency dispatch.
- A report will be distinct from a master incident, and an incident will be distinct from an assignment. None of those objects exist yet, and the schema does not collapse them into one table.
- Categories will be database records. Stage 1 has no category enum in Go or in the frontend.
- WhatsApp and SMS require opt-in. Delivery is not claimed, because nothing is sent.
- Demo data is labeled. Empty states stay empty.

## Stage 1 experience

A resident can create an account with email and a password of at least 10 characters, choose English or Hindi for the interface language, and sign in. Email is not verified; the registration page says that. They can update their name and language, save an address without a map point, opt in to WhatsApp without a message being sent, and submit an account-deletion request.

A resident can create an organization and become its owner, then add a member by email. A provider registration creates a service-provider organization. An organization registration creates the named tenant. Another resident cannot read or manage that tenant.

A support agent can list users and audit records and cannot grant roles. A super administrator can grant `support_agent` or `super_admin`. The admin navigation item is visible only for those two roles. Opening `/admin` as a resident shows the API denial.

Nearby, My Reports, and Notifications explain that those workflows are later stages.

## Still required for the pilot

Universal reporting, configurable taxonomy, incident correlation, resolver eligibility, assignment lifecycle, evidence, user confirmation, notifications with real delivery state, and operator review. The sequence is [build-checklist.md](build-checklist.md). A credible pilot also needs real resolvers in one operating area. Software cannot create that network.
