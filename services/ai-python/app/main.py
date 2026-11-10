from fastapi import FastAPI

from app.api.intake import router as intake_router

app = FastAPI(title="LokaSolve AI", version="0.1.0")
app.include_router(intake_router)


@app.get("/healthz")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/readyz")
def ready() -> dict[str, str]:
    return {"status": "ready", "model": "not_configured"}
