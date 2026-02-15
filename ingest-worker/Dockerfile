# ingest-worker Dockerfile — Python worker for Go-orchestrated ingestion (spec v2.3 §7)
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

# Install worker-specific deps
COPY ingest-worker/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt

# Copy the worker app
COPY ingest-worker/app.py /app/app.py

EXPOSE 8002

CMD ["python", "app.py"]
