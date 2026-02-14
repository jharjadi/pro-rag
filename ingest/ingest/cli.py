"""CLI entry point for pro-rag ingestion pipeline."""

from __future__ import annotations

import logging
import sys

import click

from ingest.config import IngestConfig


def _setup_logging(verbose: bool = False) -> None:
    """Configure structured logging."""
    level = logging.DEBUG if verbose else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)s %(name)s — %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%S",
        stream=sys.stderr,
    )


@click.group()
@click.option("--verbose", "-v", is_flag=True, help="Enable debug logging")
def main(verbose: bool) -> None:
    """pro-rag ingestion pipeline."""
    _setup_logging(verbose)


@main.command()
@click.argument("file_path", type=click.Path(exists=True))
@click.option("--tenant-id", required=True, help="Tenant UUID")
@click.option("--title", required=True, help="Document title")
@click.option("--activate/--no-activate", default=True, help="Activate version immediately")
def ingest(file_path: str, tenant_id: str, title: str, activate: bool) -> None:
    """Ingest a document into the knowledge base."""
    from ingest.pipeline import ingest_document

    config = IngestConfig.from_env()

    click.echo(f"Ingesting {file_path} for tenant {tenant_id}...")
    try:
        result = ingest_document(
            file_path=file_path,
            tenant_id=tenant_id,
            title=title,
            activate=activate,
            config=config,
        )

        if result["skipped"]:
            click.echo(f"⏭  Skipped (already ingested): doc_id={result['doc_id']}")
        else:
            click.echo(
                f"✅ Ingested: doc_id={result['doc_id']}, "
                f"version={result['doc_version_id']}, "
                f"chunks={result['num_chunks']}"
            )

    except FileNotFoundError as e:
        click.echo(f"❌ File not found: {e}", err=True)
        sys.exit(1)
    except ValueError as e:
        click.echo(f"❌ Invalid input: {e}", err=True)
        sys.exit(1)
    except Exception as e:
        click.echo(f"❌ Ingestion failed: {e}", err=True)
        sys.exit(1)


@main.command()
@click.option("--tenant-id", required=True, help="Tenant UUID")
@click.option("--doc-version-id", required=True, help="Document version UUID to activate")
def activate(tenant_id: str, doc_version_id: str) -> None:
    """Activate a staged document version."""
    from ingest.db.writer import get_connection

    config = IngestConfig.from_env()
    conn = get_connection(config.database_url)

    try:
        with conn.cursor() as cur:
            # Get the doc_id for this version
            cur.execute(
                "SELECT doc_id FROM document_versions WHERE doc_version_id = %s AND tenant_id = %s",
                (doc_version_id, tenant_id),
            )
            row = cur.fetchone()
            if row is None:
                click.echo(f"❌ Version not found: {doc_version_id}", err=True)
                sys.exit(1)

            doc_id = row[0]

            # Deactivate current active version
            cur.execute(
                """
                UPDATE document_versions
                SET is_active = false
                WHERE doc_id = %s AND tenant_id = %s AND is_active = true
                """,
                (doc_id, tenant_id),
            )

            # Activate the specified version
            cur.execute(
                """
                UPDATE document_versions
                SET is_active = true
                WHERE doc_version_id = %s AND tenant_id = %s
                """,
                (doc_version_id, tenant_id),
            )

            conn.commit()
            click.echo(f"✅ Activated version {doc_version_id} for doc {doc_id}")

    except Exception as e:
        conn.rollback()
        click.echo(f"❌ Activation failed: {e}", err=True)
        sys.exit(1)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
