from typing import Literal

from pydantic import BaseModel, Field


class LocationInput(BaseModel):
    latitude: float = Field(ge=-90, le=90)
    longitude: float = Field(ge=-180, le=180)
    source: Literal["user_gps", "user_pin", "user_address", "user_locality"]


class IntakeRequest(BaseModel):
    text: str = Field(min_length=1, max_length=8000)
    location: LocationInput | None = None


class Provenance(BaseModel):
    field: str
    source: Literal["user", "deterministic_rule", "official_source"]
    detail: str


class EmergencyContact(BaseModel):
    name: str
    number: str
    source_url: str
    last_verified: str


class IntakeResponse(BaseModel):
    summary: str | None
    suggested_category_id: str | None
    location_candidates: list[dict]
    missing_fields: list[str]
    risk_flags: list[str]
    recommended_action: Literal["continue", "safety_guidance"]
    user_message: str | None
    emergency_contacts: list[EmergencyContact]
    confidence: float
    provenance: list[Provenance]
    model: dict
