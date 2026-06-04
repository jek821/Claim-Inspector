package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"factchecker/internal/auth"
	"factchecker/internal/batch"
	"factchecker/internal/claims"
	"factchecker/internal/convert"
	"factchecker/internal/pricing"
	"factchecker/internal/scorer"
	"factchecker/internal/sources"
	"factchecker/internal/store"
	"factchecker/internal/types"
)

const maxUploadBytes = 10 << 20 // 10 MB

// jobStore holds in-progress and completed async batch jobs (in-memory only;
// once a job is done it's persisted via the store package).
type jobStore struct {
	mu   sync.RWMutex
	jobs map[string]*types.BatchStatusResponse
}

func newJobStore() *jobStore { return &jobStore{jobs: make(map[string]*types.BatchStatusResponse)} }
func (s *jobStore) set(id string, v *types.BatchStatusResponse) {
	s.mu.Lock(); s.jobs[id] = v; s.mu.Unlock()
}
func (s *jobStore) get(id string) (*types.BatchStatusResponse, bool) {
	s.mu.RLock(); defer s.mu.RUnlock(); v, ok := s.jobs[id]; return v, ok
}

// Handler holds all dependencies for the HTTP API.
type Handler struct {
	auth       *auth.Manager
	extractor  *claims.Extractor
	fetcher    *sources.Fetcher
	scorer     *scorer.Scorer
	batchCli   *batch.Client
	jobs       *jobStore
	store      *store.Store
	maxCostUSD float64 // 0 = unlimited
}

func NewHandler(authMgr *auth.Manager, anthropicKey string, maxCostUSD float64, st *store.Store) *Handler {
	return &Handler{
		auth:       authMgr,
		extractor:  claims.NewExtractor(anthropicKey),
		fetcher:    sources.NewFetcher(anthropicKey),
		scorer:     scorer.NewScorer(anthropicKey),
		batchCli:   batch.NewClient(anthropicKey),
		jobs:       newJobStore(),
		store:      st,
		maxCostUSD: maxCostUSD,
	}
}

// costLimitOK returns true when the request may proceed. If the configured
// spending cap is set and this request would exceed it, it writes a 402
// response and returns false.
func (h *Handler) costLimitOK(w http.ResponseWriter, text string) bool {
	if h.maxCostUSD <= 0 {
		return true
	}
	_, _, spent := h.store.GetTotals()
	if spent >= h.maxCostUSD {
		http.Error(w, fmt.Sprintf("cost limit of $%.4f reached (spent $%.4f)", h.maxCostUSD, spent), http.StatusPaymentRequired)
		return false
	}
	remaining := h.maxCostUSD - spent
	_, _, _, estCost, _ := pricing.EstimateFromText(text)
	if estCost > remaining {
		http.Error(w,
			fmt.Sprintf("request would exceed cost limit: ~$%.4f estimated, $%.4f remaining of $%.4f budget", estCost, remaining, h.maxCostUSD),
			http.StatusPaymentRequired)
		return false
	}
	return true
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/login", h.Login)

	protected := http.NewServeMux()
	protected.HandleFunc("/analyze", h.Analyze)
	protected.HandleFunc("/analyze/file", h.AnalyzeFile)
	protected.HandleFunc("/analyze/stream", h.AnalyzeStream)
	protected.HandleFunc("/estimate", h.Estimate)
	protected.HandleFunc("/batch/", h.BatchStatus)
	protected.HandleFunc("/history", h.History)
	protected.HandleFunc("/history/", h.HistoryItem) // PATCH /history/:id for rename
	protected.HandleFunc("/logout", h.Logout)

	for _, path := range []string{"/analyze", "/analyze/file", "/analyze/stream", "/estimate", "/batch/", "/history", "/history/", "/logout"} {
		mux.Handle(path, h.auth.Middleware(protected))
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	var req types.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest); return
	}
	token, ok := h.auth.Login(req.Username, req.Password)
	if !ok { http.Error(w, "invalid credentials", http.StatusUnauthorized); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.LoginResponse{Token: token})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.Logout(tokenFromRequest(r)); w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Estimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	var req types.EstimateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		http.Error(w, "invalid body", http.StatusBadRequest); return
	}
	estClaims, estInput, estOutput, costReg, costBatch := pricing.EstimateFromText(req.Text)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.CostEstimate{
		EstimatedClaims: estClaims, EstInputTokens: estInput, EstOutputTokens: estOutput,
		EstCostUSD: costReg, EstCostBatchUSD: costBatch, Model: pricing.ModelName,
	})
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	runs := h.store.GetHistory()
	in, out, total := h.store.GetTotals()
	resp := types.HistoryResponse{TotalInputTok: in, TotalOutputTok: out, TotalCostUSD: total}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, types.HistoryRun{
			ID: run.ID, CreatedAt: run.CreatedAt.Format(time.RFC3339),
			Title: run.Title, Label: run.Label, FileName: run.FileName,
			Mode: run.Mode, Claims: run.Claims, Cost: run.Cost,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HistoryItem handles PATCH /history/:id for renaming a run.
func (h *Handler) HistoryItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/history/")
	if id == "" { http.Error(w, "missing run id", http.StatusBadRequest); return }

	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	var req types.RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest); return
	}
	found, err := h.store.RenameRun(id, strings.TrimSpace(req.Label))
	if err != nil { http.Error(w, "store error", http.StatusInternalServerError); return }
	if !found { http.Error(w, "run not found", http.StatusNotFound); return }
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	var req types.AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest); return
	}
	if !h.costLimitOK(w, req.Text) { return }
	if req.Batch {
		h.submitBatch(w, req.Text, req.Label, req.FileName)
	} else {
		h.processSync(w, req.Text, req.Label, req.FileName)
	}
}

func (h *Handler) AnalyzeFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	text, filename, err := h.readFileUpload(w, r)
	if err != nil { return }
	if !h.costLimitOK(w, text) { return }
	label := r.FormValue("label")
	if r.FormValue("batch") == "true" {
		h.submitBatch(w, text, label, filename)
	} else {
		h.processSync(w, text, label, filename)
	}
}

// AnalyzeStream is the SSE endpoint for real-time progress during sync analysis.
// GET /analyze/stream?text=... OR POST with body (text sent via query param or body).
func (h *Handler) AnalyzeStream(w http.ResponseWriter, r *http.Request) {
	// Accept text from POST body
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	var req types.AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest); return
	}
	if !h.costLimitOK(w, req.Text) { return }

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok { http.Error(w, "streaming not supported", http.StatusInternalServerError); return }

	sendEvent := func(eventType string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
		flusher.Flush()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	sendEvent("status", map[string]string{"message": "Extracting claims…"})
	extracted, err := h.extractor.Extract(ctx, req.Text)
	if err != nil {
		sendEvent("error", map[string]string{"message": "Failed to extract claims: " + err.Error()}); return
	}

	total := len(extracted.Claims)
	sendEvent("extracted", map[string]int{"total": total})

	results := make([]types.Claim, total)
	var done atomic.Int32
	totalInput := extracted.InputTokens; totalOutput := extracted.OutputTokens
	var mu sync.Mutex
	progressCh := make(chan types.ProgressEvent, total)

	var wg sync.WaitGroup
	for i, claimText := range extracted.Claims {
		wg.Add(1)
		go func(idx int, ct string) {
			defer wg.Done()
			srcs := h.fetcher.FetchAll(ctx, ct)
			sr, err := h.scorer.Score(ctx, ct, srcs)
			if err != nil { sr.Risk = "unverifiable"; sr.Explanation = "Could not complete scoring." }
			mu.Lock(); totalInput += sr.InputTokens; totalOutput += sr.OutputTokens; mu.Unlock()
			results[idx] = types.Claim{Text: ct, Risk: sr.Risk, Explanation: sr.Explanation, Sources: srcs}
			n := int(done.Add(1))
			snippet := ct; if len(snippet) > 60 { snippet = snippet[:60] + "…" }
			progressCh <- types.ProgressEvent{Done: n, Total: total, Current: snippet}
		}(i, claimText)
	}

	go func() { wg.Wait(); close(progressCh) }()
	for evt := range progressCh { sendEvent("progress", evt) }

	cost := types.CostBreakdown{
		Model: pricing.ModelName,
		Usage: types.TokenUsage{InputTokens: totalInput, OutputTokens: totalOutput},
		ExactCostUSD: pricing.CalcExact(totalInput, totalOutput),
	}
	h.persistRun(req.Text, req.Label, req.FileName, "sync", results, cost)
	sendEvent("done", types.AnalyzeResponse{Claims: results, Cost: cost})
}

func (h *Handler) processSync(w http.ResponseWriter, text, label, filename string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	extracted, err := h.extractor.Extract(ctx, text)
	if err != nil {
		log.Printf("extract error: %v", err)
		http.Error(w, "failed to extract claims", http.StatusInternalServerError); return
	}

	totalInput := extracted.InputTokens; totalOutput := extracted.OutputTokens
	var mu sync.Mutex
	results := make([]types.Claim, len(extracted.Claims))
	var wg sync.WaitGroup

	for i, ct := range extracted.Claims {
		wg.Add(1)
		go func(idx int, claimText string) {
			defer wg.Done()
			srcs := h.fetcher.FetchAll(ctx, claimText)
			sr, err := h.scorer.Score(ctx, claimText, srcs)
			if err != nil { sr.Risk = "unverifiable"; sr.Explanation = "Could not complete scoring." }
			mu.Lock(); totalInput += sr.InputTokens; totalOutput += sr.OutputTokens; mu.Unlock()
			results[idx] = types.Claim{Text: claimText, Risk: sr.Risk, Explanation: sr.Explanation, Sources: srcs}
		}(i, ct)
	}
	wg.Wait()

	cost := types.CostBreakdown{
		Model: pricing.ModelName,
		Usage: types.TokenUsage{InputTokens: totalInput, OutputTokens: totalOutput},
		ExactCostUSD: pricing.CalcExact(totalInput, totalOutput),
	}
	h.persistRun(text, label, filename, "sync", results, cost)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.AnalyzeResponse{Claims: results, Cost: cost})
}

// ── Async Batch API ───────────────────────────────────────────────────────────

func (h *Handler) submitBatch(w http.ResponseWriter, text, label, filename string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	extracted, err := h.extractor.Extract(ctx, text)
	if err != nil {
		log.Printf("extract error: %v", err)
		http.Error(w, "failed to extract claims", http.StatusInternalServerError); return
	}

	claimSources := make([][]types.Source, len(extracted.Claims))
	var wg sync.WaitGroup
	for i, ct := range extracted.Claims {
		wg.Add(1)
		go func(idx int, claim string) {
			defer wg.Done()
			claimSources[idx] = h.fetcher.FetchAll(ctx, claim)
		}(i, ct)
	}
	wg.Wait()

	requests := make([]batch.Request, len(extracted.Claims))
	for i, ct := range extracted.Claims {
		requests[i] = batch.Request{
			CustomID: fmt.Sprintf("claim-%d", i),
			Params:   h.scorer.BuildParams(ct, claimSources[i]),
		}
	}

	batchCtx, batchCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer batchCancel()

	batchID, err := h.batchCli.Submit(batchCtx, requests)
	if err != nil {
		log.Printf("batch submit error: %v", err)
		http.Error(w, "failed to submit batch", http.StatusInternalServerError); return
	}

	h.jobs.set(batchID, &types.BatchStatusResponse{
		BatchID: batchID, Status: "processing", Total: len(extracted.Claims),
	})
	go h.pollBatch(batchID, text, label, filename, extracted.Claims, claimSources, extracted.InputTokens, extracted.OutputTokens)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.BatchSubmitResponse{
		BatchID: batchID,
		Message: fmt.Sprintf("Batch submitted with %d claims. Poll GET /batch/%s for results.", len(extracted.Claims), batchID),
	})
}

func (h *Handler) BatchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	id := strings.TrimPrefix(r.URL.Path, "/batch/")
	if id == "" { http.Error(w, "missing batch id", http.StatusBadRequest); return }
	status, ok := h.jobs.get(id)
	if !ok { http.Error(w, "batch not found", http.StatusNotFound); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *Handler) pollBatch(jobID, text, label, filename string, claimTexts []string, claimSources [][]types.Source, extractIn, extractOut int) {
	ctx := context.Background()
	finalStatus, err := h.batchCli.Poll(ctx, jobID, func(succeeded, total int) {
		progress := 0
		if total > 0 { progress = succeeded * 100 / total }
		h.jobs.set(jobID, &types.BatchStatusResponse{
			BatchID: jobID, Status: "processing",
			Progress: progress, Succeeded: succeeded, Total: total,
		})
	})
	if err != nil {
		h.jobs.set(jobID, &types.BatchStatusResponse{BatchID: jobID, Status: "failed", Error: err.Error()}); return
	}

	resultMap, err := h.batchCli.Results(ctx, finalStatus.ID)
	if err != nil {
		h.jobs.set(jobID, &types.BatchStatusResponse{BatchID: jobID, Status: "failed", Error: err.Error()}); return
	}

	claimsOut := make([]types.Claim, len(claimTexts))
	totalInput := extractIn; totalOutput := extractOut

	for i, ct := range claimTexts {
		item, ok := resultMap[fmt.Sprintf("claim-%d", i)]
		if !ok || item.Result.Type != "succeeded" || item.Result.Message == nil {
			claimsOut[i] = types.Claim{Text: ct, Risk: "unverifiable", Explanation: "Batch result unavailable.", Sources: claimSources[i]}
			continue
		}
		var rawText string
		for _, c := range item.Result.Message.Content {
			if c.Type == "text" { rawText = c.Text; break }
		}
		risk, explanation := h.scorer.ParseScoreText(rawText)
		totalInput += item.Result.Message.Usage.InputTokens
		totalOutput += item.Result.Message.Usage.OutputTokens
		claimsOut[i] = types.Claim{Text: ct, Risk: risk, Explanation: explanation, Sources: claimSources[i]}
	}

	cost := &types.CostBreakdown{
		Model: pricing.ModelName,
		Usage: types.TokenUsage{InputTokens: totalInput, OutputTokens: totalOutput},
		ExactCostUSD: pricing.CalcExact(totalInput, totalOutput),
	}
	h.persistRun(text, label, filename, "batch", claimsOut, *cost)
	h.jobs.set(jobID, &types.BatchStatusResponse{
		BatchID: jobID, Status: "done", Progress: 100,
		Succeeded: len(claimTexts), Total: len(claimTexts),
		Claims: claimsOut, Cost: cost,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) readFileUpload(w http.ResponseWriter, r *http.Request) (text, filename string, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err = r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "file too large (max 10MB)", http.StatusRequestEntityTooLarge); return
	}
	file, header, ferr := r.FormFile("file")
	if ferr != nil { http.Error(w, "missing file field", http.StatusBadRequest); err = ferr; return }
	defer file.Close()

	raw, rerr := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if rerr != nil { http.Error(w, "failed to read file", http.StatusInternalServerError); err = rerr; return }

	filename = header.Filename
	text, err = convert.FromBytes(raw, convert.ExtFromFilename(filename))
	if err != nil {
		status := http.StatusBadRequest
		if err == convert.ErrScannedPDF { status = http.StatusUnprocessableEntity }
		http.Error(w, err.Error(), status); return
	}
	if strings.TrimSpace(text) == "" {
		http.Error(w, "no text could be extracted from the file", http.StatusBadRequest)
		err = fmt.Errorf("empty"); return
	}
	return
}

func (h *Handler) persistRun(text, label, filename, mode string, claimsOut []types.Claim, cost types.CostBreakdown) {
	title := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if len(title) > 80 { title = title[:80] + "…" }
	run := store.Run{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		CreatedAt: time.Now(),
		Title:     title,
		Label:     strings.TrimSpace(label),
		FileName:  filename,
		Mode:      mode,
		Claims:    claimsOut,
		Cost:      cost,
	}
	if err := h.store.AddRun(run); err != nil {
		log.Printf("store error: %v", err)
	}
}

func tokenFromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " { return h[7:] }
	return ""
}
