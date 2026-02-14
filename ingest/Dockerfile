# ingest Dockerfile — Python ingestion pipeline
FROM python:3.11-slim

WORKDIR /app

# Install system deps for psycopg2, pdfplumber, pymupdf
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml .
RUN pip install --no-cache-dir -e ".[dev]" 2>/dev/null || pip install --no-cache-dir .

COPY . .

ENTRYPOINT ["prorag-ingest"]
