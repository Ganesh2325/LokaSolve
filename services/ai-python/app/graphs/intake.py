import re

from app.safety.directory import INDIA_EMERGENCY, INDIA_MENTAL_HEALTH
from app.schemas.intake import IntakeRequest, IntakeResponse, Provenance

EMERGENCY = re.compile(
    r"\b(gas leak|electrocution|electric shock|live wire|explosion|not breathing|"
    r"unconscious|heart attack|serious injury|bleeding heavily|gunshot|stabbed|"
    r"building fire|house fire|on fire|caught fire)\b",
    re.IGNORECASE,
)
CRISIS = re.compile(
    r"(\bsuicid|\bkill myself\b|\bend my life\b|\bself[- ]harm\b|\bwant to die\b)",
    re.IGNORECASE,
)

MODEL = {
    "provider": "none",
    "name": "stage1-deterministic-skeleton",
    "prompt_version": "0",
}


def understand(request: IntakeRequest) -> IntakeResponse:
    """Classify only what the caller explicitly supplied.

    This stage does not call a model and does not infer coordinates, addresses,
    or a category. A later stage can add a model behind the same response schema.
    """

    provenance = [
        Provenance(
            field="summary",
            source="deterministic_rule",
            detail="No model is configured, so no summary was inferred.",
        )
    ]
    missing = ["category"]
    candidates: list[dict] = []
    if request.location is None:
        missing.append("location")
        provenance.append(
            Provenance(
                field="location_candidates",
                source="deterministic_rule",
                detail="No coordinates or address object were provided; none were inferred.",
            )
        )
    else:
        candidates.append(
            {
                "latitude": request.location.latitude,
                "longitude": request.location.longitude,
                "source": "user",
                "confidence": 1,
            }
        )
        provenance.append(
            Provenance(
                field="location_candidates",
                source="user",
                detail="Coordinates were copied from the explicit location object.",
            )
        )

    flags: list[str] = []
    contacts = []
    message = None
    action = "continue"
    if EMERGENCY.search(request.text):
        flags.append("possible_immediate_danger")
        action = "safety_guidance"
        contacts = [INDIA_EMERGENCY]
        message = (
            "This may be an immediate danger. Call 112 now. Do not wait for LokaSolve "
            "or try to repair it yourself."
        )
    elif CRISIS.search(request.text):
        flags.append("possible_personal_crisis")
        action = "safety_guidance"
        contacts = [INDIA_MENTAL_HEALTH, INDIA_EMERGENCY]
        message = (
            "If you are in immediate danger, call 112. For mental-health support in India, "
            "Tele-MANAS is available at 14416. Do not wait for LokaSolve."
        )

    return IntakeResponse(
        summary=None,
        suggested_category_id=None,
        location_candidates=candidates,
        missing_fields=missing,
        risk_flags=flags,
        recommended_action=action,
        user_message=message,
        emergency_contacts=contacts,
        confidence=0,
        provenance=provenance,
        model=MODEL,
    )
