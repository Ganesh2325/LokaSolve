"""Verified public emergency contacts.

Numbers below were checked against official public pages on 2026-11-08.
They are guidance for the person making a report. LokaSolve does not dispatch
these services and must not say that help has been sent.
"""

from app.schemas.intake import EmergencyContact

INDIA_EMERGENCY = EmergencyContact(
    name="Emergency Response Support System",
    number="112",
    source_url="https://www.112.gov.in/",
    last_verified="2026-11-08",
)

INDIA_MENTAL_HEALTH = EmergencyContact(
    name="Tele-MANAS",
    number="14416",
    source_url="https://telemanas.mohfw.gov.in/",
    last_verified="2026-11-08",
)
