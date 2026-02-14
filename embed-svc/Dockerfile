FROM python:3.11-slim

WORKDIR /app

# Install dependencies
RUN pip install --no-cache-dir flask sentence-transformers

COPY app.py .

# Pre-download model at build time for faster startup
RUN python -c "from sentence_transformers import SentenceTransformer; SentenceTransformer('BAAI/bge-base-en-v1.5')"

EXPOSE 8001

CMD ["python", "app.py"]
