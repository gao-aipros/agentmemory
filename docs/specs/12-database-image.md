# v2 Database: ParadeDB Docker Image

## Image

- Official Docker image: `paradedb/paradedb`
- Pre-installed extensions: pg_search (BM25), pgvector (HNSW), postgis, pg_ivm, pg_cron
- **Only pg_search and pgvector are enabled** — others not created to keep minimal footprint
- Quick start:
  ```
  docker run -e POSTGRES_USER=myuser -e POSTGRES_PASSWORD=mypassword \
    -e POSTGRES_DB=mydb -p 5432:5432 paradedb/paradedb:latest
  ```

## Graph Traversal

- Apache AGE is **not** in the ParadeDB image
- v2 uses PostgreSQL native `WITH RECURSIVE` CTE for graph traversal
- AGE may be considered later (would require custom Docker image)

## Extensions in Schema

```sql
CREATE EXTENSION IF NOT EXISTS pg_search;
CREATE EXTENSION IF NOT EXISTS vector;
```
