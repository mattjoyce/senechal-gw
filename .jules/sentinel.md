## 2026-05-13 - [Fix CORS Wildcard Misconfiguration]
**Vulnerability:** The API server's `corsMiddleware` in `internal/api/server.go` explicitly reflected any incoming `Origin` and unconditionally set `Access-Control-Allow-Credentials: true`.
**Learning:** This is a critical security risk because combining a reflected wildcard origin with `true` credentials allows arbitrary websites to make authenticated requests on behalf of the user to the server and read sensitive responses.
**Prevention:** Always implement configurable CORS policies (`CORSConfig` via `config.yaml`). Default to wildcard origins but force credentials to `false`. If specific origins require credentials, ensure the configured `AllowedOrigins` explicitly match the requested origin without wildcards.
