# Papierkram API spec snapshot

| | |
|---|---|
| File | `openapi.json` (unmodified) |
| Source | https://demo.papierkram.de/api/v1/api-docs/api/v1/swagger.json |
| Swagger UI | https://demo.papierkram.de/api/v1/api-docs/index.html |
| Retrieved | 2026-11-03 |
| Format | OpenAPI 3.0.3, `info.version` = `v1` |
| Size | 499 181 bytes |
| SHA-256 | `da826bc0bd364c1749ae088d42287b818b99e8c536ca5ced61193f2581e9ccea` |
| Operations | 89 |

The base URL in the spec (`https://demo.papierkram.de/api/v1`) is instance-specific. Real accounts use `https://<subdomain>.papierkram.de/api/v1`.

Never edit `openapi.json` by hand. Fixes needed for code generation go into `overlay.yaml` (added during implementation). To refresh the snapshot, download it again with `curl`, update this table, and review the diff.

## Known defects in the spec

These are handled by the overlay and the client, not by editing the file:

- No `operationId` on any operation.
- All request bodies are inline schemas; there is no shared error schema and no response headers (the quota headers `X-Remaining-Quota` / `X-Consumed-Quota` are undocumented).
- PDF endpoints use the literal content-type key `["application/pdf"]` and reference a `*List` schema.
- `deliver` documents `201` with an object body; the live API answers 2xx with an empty body.
- `order_direction` is typed as an array of `asc|desc`; the API takes a single value.
- `send_via` and several other enums contain `null`; `difference_reason` contains `sonstige` twice; the expense `category` enum has 99 entries, 94 unique.
