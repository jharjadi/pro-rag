# ingest-api Dockerfile — FastAPI wrapper around ingestion pipeline
# Build context must be the project root (set in docker-compose.yml)
FROM python:3.11-slim

WORKDIR /app

# Install system deps for psycopg2, pdfplumber, pymupdf
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# Install ingest library first (shared with CLI)
COPY ingest/ /app/ingest-lib/
RUN pip install --no-cache-dir /app/ingest-lib

# Install API-specific deps
COPY ingest-api/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt

# Copy the API app
COPY ingest-api/app.py /app/app.py

EXPOSE 8002

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8002"]
