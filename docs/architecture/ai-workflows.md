# AI workflows

This describes the deterministic skeleton that is running today. The target is a Python LangGraph service that calls other services over HTTP, as recorded in [target services](target-services.md).

Stage 1 does not run a model and does not use LangGraph. `POST /v1/intake/understand` on the Python service is a deterministic function with the response shape later model nodes must keep.

## What the skeleton does

- It leaves `summary` empty and records provenance `deterministic_rule` saying no summary was inferred.
- If the request has no location object, `location_candidates` is empty and `missing_fields` includes `location`. Coordinates are never guessed from an address or from the text.
- If the request includes a location, those latitude and longitude values are copied with source `user` and confidence 1.
- `missing_fields` always includes `category`, because no taxonomy is selected in this stage.
- `recommended_action` is `continue` unless the text matches the local danger or crisis patterns.

## Safety guidance

The directory in `services/ai-python/app/safety/directory.py` stores numbers that were checked on 2026-11-08:

- India emergency response (ERSS): 112, from the Ministry of Home Affairs site `https://www.112.gov.in/`.
- Tele-MANAS: 14416, from the Directorate General of Health Services site `https://telemanas.mohfw.gov.in/`. The same service also publishes toll-free numbers 1800-891-4416 and 1800-89-14416. The directory currently returns 14416.

For gas-leak style text the response sets `recommended_action` to `safety_guidance`, adds the 112 contact, and tells the person to call 112 and not to wait for LokaSolve or repair the hazard. Crisis language returns Tele-MANAS 14416 and 112. The service does not say that emergency help was dispatched, and it does not give repair steps.

These patterns are a narrow keyword gate, not a clinical or emergency classifier. They err toward showing the directory when the configured phrases match. They will miss paraphrases. A later model must keep the same rule: low confidence escalates, and ordinary provider matching is not offered for immediate danger.

## Rules that later graph nodes inherit

- Model output is validated before it can change domain state. This stage's contract is the Pydantic models in `app/schemas/intake.py`.
- Facts and inferences stay distinct. Provenance names the source.
- The graph checkpoint, when it exists, is workflow state. Issues, incidents, assignments, and permissions stay in PostgreSQL.
- The model cannot approve payment, submit an official complaint, change privacy settings, or dispatch emergency services.
- `ai_intake` is a feature flag and it is off.

The Go API does not call the Python service yet. Running the skeleton does not classify a stored report, because reports do not exist.
