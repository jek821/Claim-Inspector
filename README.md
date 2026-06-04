# Claim Inspector

An open-source fact-checking tool that cross-references text against Wikipedia, Semantic Scholar, arXiv, PubMed, and NewsAPI, then uses Claude Haiku to score each claim by accuracy risk.

Paste text or upload a document — claims are highlighted green/yellow/orange/red based on how suspect they are, with source links for every finding. Full run history and cost tracking persist across server restarts.

---

## How it works

1. Text is split into atomic factual claims (one Haiku API call for the whole document)
2. Each claim is searched concurrently against Wikipedia, Semantic Scholar, arXiv, and PubMed (plus NewsAPI if configured)
3. Retrieved sources are passed to Haiku to score each claim: `verified / low / medium / high / unverifiable`
4. Results stream back in real-time with color-coded highlights and clickable source links
5. Every completed run is saved to disk — history and cumulative costs survive server restarts

### Instant mode vs Batch mode

| | Instant | Batch |
|---|---|---|
| How it works | Claims scored concurrently, results stream live via SSE | All scoring requests submitted to Anthropic's async Batch API at once |
| Speed | Seconds to a few minutes | Up to 24 hours (often minutes in practice) |
| Cost | Standard Haiku pricing | **50% cheaper** — Anthropic's batch discount |
| Rate limits | Can hit limits on very large docs | No rate limit concerns |
| Results | Streamed to browser in real-time | Polled every 8 seconds, saved to disk — safe to close tab |

---

## Supported file formats

Upload directly — the backend converts everything to plain text before processing:

| Format | Notes |
|---|---|
| `.txt` | Lowest token overhead — cheapest to process |
| `.md` | Markdown syntax stripped before sending |
| `.html` | Tags, scripts, and styles stripped |
| `.docx` | Word documents — text extracted from XML internals, no external library needed |
| `.pdf` | Text-based PDFs only via `pdftotext`. **Scanned/image PDFs are not supported** |

To minimize cost, strip headers, footers, citations, and page numbers before uploading — they count as tokens but rarely contain checkable claims.

---

## Stack

- **Backend** — Go 1.22+ (stdlib only, no frameworks)
- **Frontend** — React + Vite, single-page app
- **AI** — Claude Haiku 4.5 via Anthropic API ($1/$5 per million input/output tokens)
- **Sources** — Wikipedia (free), Semantic Scholar (free), arXiv (free), PubMed/NCBI E-utilities (free), NewsAPI (optional, free tier: 100 req/day)
- **Persistence** — JSON file on disk (`data/history.json`), atomic writes

---

## Prerequisites

Before deploying, you'll need:

- A Linux server (Ubuntu 20.04+ recommended) with SSH access
- Go 1.22+ — install at https://go.dev/dl
- Node.js 18+ and npm — for building the frontend
- nginx — `sudo apt install nginx`
- poppler-utils — for PDF support: `sudo apt install poppler-utils`
- An **Anthropic API key** — get one at https://console.anthropic.com
- *(Optional)* A **NewsAPI key** — free tier at https://newsapi.org (100 req/day). Without it the tool works via Wikipedia, Semantic Scholar, arXiv, and PubMed.

---

## Environment variables

Copy `backend/.env.example` to `backend/.env` and fill in your values:

```bash
cp backend/.env.example backend/.env
```

### Required

| Variable | Description |
|---|---|
| `ANTHROPIC_API_KEY` | Your Anthropic API key. Get one at console.anthropic.com |
| `APP_USERNAME` | Login username for the web interface |
| `APP_PASSWORD` | Login password for the web interface. Use something strong — this is the only thing protecting your API key |

### Optional

| Variable | Default | Description |
|---|---|---|
| `NEWS_API_KEY` | *(empty)* | NewsAPI key for news source cross-referencing. Tool works without it |
| `PORT` | `8080` | Port the backend listens on |
| `DATA_DIR` | `data` | Directory where `history.json` is stored. Set to an absolute path on your VPS so history survives redeploys (e.g. `/var/lib/factchecker`) |

### Example `.env`

```
ANTHROPIC_API_KEY=sk-ant-api03-...
APP_USERNAME=jacob
APP_PASSWORD=some-long-random-password
NEWS_API_KEY=abc123yourkeyhere
PORT=8080
DATA_DIR=/var/lib/factchecker
```

**Never commit your `.env` file.** It's already in `.gitignore`.

---

## Deployment

### 1. Clone the repo

```bash
git clone https://github.com/yourname/claim-inspector.git
cd claim-inspector
```

### 2. Build the frontend

```bash
cd frontend
npm install

# Set your backend URL — use your domain or server IP
echo "VITE_API_URL=https://yourdomain.com" > .env

npm run build
# Output is in frontend/dist/
```

### 3. Build the backend

Build on your server directly:

```bash
cd backend
go build -o factchecker-server ./cmd/server
```

Or cross-compile on your local machine and copy the binary over:

```bash
# On your local machine (targeting Linux x86-64)
cd backend
GOOS=linux GOARCH=amd64 go build -o factchecker-server ./cmd/server

# Copy to server
scp factchecker-server user@yourserver:/usr/local/bin/factchecker-server
```

### 4. Create your `.env` and data directory on the server

```bash
# Create config directory
sudo mkdir -p /etc/factchecker
sudo nano /etc/factchecker/.env
# Fill in your variables (see above)
sudo chmod 600 /etc/factchecker/.env

# Create data directory for history persistence
sudo mkdir -p /var/lib/factchecker
sudo chown www-data:www-data /var/lib/factchecker
```

### 5. Set up the systemd service

Create `/etc/systemd/system/factchecker.service`:

```ini
[Unit]
Description=Claim Inspector Backend
After=network.target

[Service]
ExecStart=/usr/local/bin/factchecker-server
WorkingDirectory=/etc/factchecker
EnvironmentFile=/etc/factchecker/.env
Restart=always
RestartSec=5
User=www-data

[Install]
WantedBy=multi-user.target
```

Enable and start it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable factchecker
sudo systemctl start factchecker

# Check it's running
sudo systemctl status factchecker
# View logs
sudo journalctl -u factchecker -f
```

### 6. Copy the frontend build

```bash
sudo mkdir -p /var/www/factchecker
sudo cp -r frontend/dist/* /var/www/factchecker/
sudo chown -R www-data:www-data /var/www/factchecker
```

### 7. Configure nginx

Create `/etc/nginx/sites-available/factchecker`:

```nginx
server {
    listen 80;
    server_name yourdomain.com;  # or your server IP

    # Serve the React frontend
    root /var/www/factchecker;
    index index.html;

    location / {
        try_files $uri /index.html;
    }

    # Proxy API requests to the Go backend
    location ~ ^/(analyze|login|logout|health|estimate|batch|history) {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;

        # Required for SSE (real-time progress streaming)
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_cache off;

        # Allow large file uploads (10MB)
        client_max_body_size 10M;

        # Long timeout for batch polling (up to 24h)
        proxy_read_timeout 86400s;
    }
}
```

Enable it:

```bash
sudo ln -s /etc/nginx/sites-available/factchecker /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### 8. (Recommended) Add HTTPS with Certbot

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d yourdomain.com
```

Certbot automatically updates your nginx config. HTTPS is required for the app to work correctly in most modern browsers, and ensures your login credentials are encrypted in transit.

---

## Running locally (development)

**Backend:**
```bash
cd backend
cp .env.example .env
# Fill in ANTHROPIC_API_KEY, APP_USERNAME, APP_PASSWORD
go run ./cmd/server
# Runs on http://localhost:8080
# History saved to ./data/history.json
```

**Frontend:**
```bash
cd frontend
cp .env.example .env
# VITE_API_URL=http://localhost:8080 is already set in the example
npm install
npm run dev
# Runs on http://localhost:5173
```

---

## Cost

All pricing is for Claude Haiku 4.5 as of May 2026. The in-app cost tracker shows exact token counts pulled from the Anthropic API response and persists all-time totals to disk.

| Volume | Instant mode | Batch mode |
|---|---|---|
| 10 pages | ~$0.01 | ~$0.005 |
| 100 pages | ~$0.09 | ~$0.045 |
| 1,000 pages | ~$0.90 | ~$0.45 |

Wikipedia, Semantic Scholar, arXiv, and PubMed are all free with no API keys required. NewsAPI free tier allows 100 requests/day — each claim uses one request, so you'll hit the limit around 100 claims/day on the free tier.

---

## API reference

All protected endpoints require `Authorization: Bearer <token>` header.

### `POST /login`
```json
{ "username": "...", "password": "..." }
→ { "token": "..." }
```

### `POST /analyze` *(auth)*
Standard JSON endpoint. Returns immediately for sync; returns `batch_id` for async batch.
```json
{ "text": "...", "batch": false }

// Sync response:
→ { "claims": [...], "cost": { "exact_cost_usd": 0.0012, "usage": { "input_tokens": 800, "output_tokens": 200 } } }

// Batch response (batch: true):
→ { "batch_id": "msgbatch_...", "message": "Batch submitted with 8 claims..." }
```

### `POST /analyze/stream` *(auth)*
SSE endpoint for real-time progress during instant mode. Returns `text/event-stream`.
```
event: extracted   data: {"total": 8}
event: progress    data: {"done": 3, "total": 8, "current": "The earliest known frog…"}
event: done        data: { "claims": [...], "cost": {...} }
event: error       data: {"message": "..."}
```

### `POST /analyze/file` *(auth)*
Multipart form upload. Fields: `file` (required), `batch` (optional, `"true"`).
Accepts `.txt`, `.md`, `.html`, `.htm`, `.docx`, `.pdf`.

### `GET /batch/:id` *(auth)*
Poll status of an async batch job.
```json
→ { "batch_id": "...", "status": "processing|done|failed", "progress": 60, "succeeded": 5, "total": 8 }
// When done, also includes: "claims": [...], "cost": {...}
```

### `POST /estimate` *(auth)*
Pre-run cost estimate — no API calls made.
```json
{ "text": "..." }
→ { "estimated_claims": 8, "est_input_tokens": 4200, "est_output_tokens": 1200, "est_cost_usd": 0.010, "est_cost_batch_usd": 0.005, "model": "claude-haiku-4-5-20251001" }
```

### `GET /history` *(auth)*
Returns all past runs and cumulative cost totals.
```json
→ {
    "runs": [{ "id": "...", "created_at": "...", "title": "...", "mode": "sync|batch", "claims": [...], "cost": {...} }],
    "total_input_tokens": 42000,
    "total_output_tokens": 8400,
    "total_cost_usd": 0.084
  }
```

### `GET /health`
```json
→ { "status": "ok" }
```

### `POST /logout` *(auth)*
Invalidates the current session token. Returns `204 No Content`.

---

## Contributing

PRs welcome. Key areas for improvement:

- PDF support for scanned documents via OCR (Tesseract)
- Better claim extraction for non-English text
- Additional source APIs (JSTOR, bioRxiv, Google Scholar scraping)
- User accounts — currently single-user only
- Export results as PDF or CSV

---

## License

MIT
