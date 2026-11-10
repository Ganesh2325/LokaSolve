from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_missing_location_is_not_invented():
    response = client.post("/v1/intake/understand", json={"text": "The street light is out"})
    assert response.status_code == 200
    body = response.json()
    assert body["location_candidates"] == []
    assert "location" in body["missing_fields"]
    assert body["suggested_category_id"] is None
    assert body["confidence"] == 0
    assert all(item["source"] != "model" for item in body["provenance"])


def test_explicit_location_is_preserved():
    response = client.post(
        "/v1/intake/understand",
        json={
            "text": "Water is leaking from the public tap",
            "location": {"latitude": 18.52, "longitude": 73.85, "source": "user_pin"},
        },
    )
    body = response.json()
    assert body["location_candidates"] == [
        {"latitude": 18.52, "longitude": 73.85, "source": "user", "confidence": 1}
    ]
    assert "location" not in body["missing_fields"]


def test_gas_leak_is_not_sent_to_ordinary_matching():
    response = client.post("/v1/intake/understand", json={"text": "There is a gas leak in the stairwell"})
    body = response.json()
    assert body["recommended_action"] == "safety_guidance"
    assert "possible_immediate_danger" in body["risk_flags"]
    assert body["emergency_contacts"][0]["number"] == "112"
    assert "Do not wait for LokaSolve" in body["user_message"]
    assert "repair" not in body["user_message"].lower() or "do not" in body["user_message"].lower()


def test_ordinary_report_is_not_treated_as_an_emergency():
    response = client.post("/v1/intake/understand", json={"text": "A pothole has opened near the bus stop"})
    body = response.json()
    assert body["recommended_action"] == "continue"
    assert body["risk_flags"] == []
    assert body["emergency_contacts"] == []
