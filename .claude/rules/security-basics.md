# Security basics

- Never commit secrets, tokens, or real `.env` values.
- Sanitize any HTML/SVG rendered from untrusted input.
- Build URLs with `URL` / `URLSearchParams`, not string concat.
