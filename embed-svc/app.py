"""Lightweight embedding HTTP sidecar for the Go query API.

Wraps sentence-transformers to provide a simple JSON API for embedding text.
Used by core-api-go to embed questions for vector search.

Endpoint: POST /embed
Request:  {"texts": ["text1", "text2"]}
Response: {"embeddings": [[0.1, 0.2, ...], [0.3, 0.4, ...]]}
"""

from __future__ import annotations

import logging
import os
import sys

from flask import Flask, jsonify, request
from sentence_transformers import SentenceTransformer

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s — %(message)s",
    stream=sys.stderr,
)
logger = logging.getLogger("embed-svc")

MODEL_NAME = os.environ.get("EMBEDDING_MODEL", "BAAI/bge-base-en-v1.5")
PORT = int(os.environ.get("EMBED_PORT", "8001"))

app = Flask(__name__)

# Load model at startup
logger.info("Loading embedding model: %s", MODEL_NAME)
model = SentenceTransformer(MODEL_NAME)
logger.info("Model loaded. Embedding dimension: %d", model.get_sentence_embedding_dimension())


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok", "model": MODEL_NAME})


@app.route("/embed", methods=["POST"])
def embed():
    data = request.get_json()
    if not data or "texts" not in data:
        return jsonify({"error": "missing 'texts' field"}), 400

    texts = data["texts"]
    if not isinstance(texts, list) or len(texts) == 0:
        return jsonify({"error": "'texts' must be a non-empty list"}), 400

    embeddings = model.encode(texts, normalize_embeddings=True)
    return jsonify({"embeddings": embeddings.tolist()})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=PORT)
