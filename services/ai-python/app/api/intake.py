from fastapi import APIRouter

from app.graphs.intake import understand
from app.schemas.intake import IntakeRequest, IntakeResponse

router = APIRouter()


@router.post("/v1/intake/understand", response_model=IntakeResponse)
def intake(request: IntakeRequest) -> IntakeResponse:
    return understand(request)
