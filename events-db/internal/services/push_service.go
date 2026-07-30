package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"events-db/internal/config"
	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ─────────────────────────────────────────────────────────────────────────────
// Expo push delivery
//
// Every notification row written by repository.CreateNotification is handed to
// Dispatch, which enqueues it. A single background sender drains the queue in
// BATCHES — this is the whole reason the service exists rather than a bare
// "POST to Expo" helper.
//
// Why batching is load-bearing: the sell-out worker fans one alert out to every
// follower of an event. For a 5 000-follower event, a naive per-notification
// implementation would make 5 000 token-resolution queries and 5 000 HTTPS
// round-trips to Expo, take minutes, and trip Expo's rate limiter. Batched, the
// same alert is ~1 token query, ~1 badge query, and 50 HTTPS calls (Expo caps a
// send request at 100 messages) — a couple of seconds.
//
// The pipeline:
//
//	Dispatch  →  queue (buffered chan)
//	          →  collect up to maxBatchJobs or flushInterval, whichever first
//	          →  resolve every recipient's devices in ONE query
//	          →  resolve unread counts for iOS badges in ONE query
//	          →  chunk to 100 messages, POST each chunk
//	          →  immediate ticket errors: DeviceNotRegistered ⇒ delete token
//	          →  ok tickets: remember receipt id, check ~15 min later
//
// DELIVERY GUARANTEE: none, deliberately. Push is best-effort notification, not
// transport. The in-app feed row is the durable record and is written first; if
// the phone was off, the token was stale, or the process restarted with jobs
// still queued, the user still sees the notification when they open the app.
// Nothing here retries across a restart, and it shouldn't — a booking
// confirmation buzzing six hours late is worse than not buzzing.
// ─────────────────────────────────────────────────────────────────────────────

const (
	expoSendURL     = "https://exp.host/--/api/v2/push/send"
	expoReceiptsURL = "https://exp.host/--/api/v2/push/getReceipts"

	// Expo's documented cap is 100 messages per send request and 1 000 ids per
	// receipts request. Exceeding either is a 400, not a partial success.
	maxMessagesPerRequest = 100
	maxReceiptsPerRequest = 1000

	// How many notification jobs one batch may pull off the queue before it
	// stops collecting and sends. Larger than maxMessagesPerRequest because one
	// job can expand to several messages (a user with a phone AND a tablet).
	maxBatchJobs = 500

	// Longest a job waits for company before being sent alone. 400ms is below
	// human perception for a "booking confirmed" push and long enough for a
	// fan-out loop to fill the batch.
	flushInterval = 400 * time.Millisecond

	// Dispatch drops jobs when the queue is full rather than blocking the
	// caller. 4096 absorbs a large fan-out; past that, the feed row still
	// exists and the drop is logged loudly.
	queueSize = 4096

	// Expo tickets aren't final — a ticket says "accepted", a receipt says
	// "delivered". Receipts are unavailable for a few minutes after send, so
	// they're checked on a delay.
	receiptCheckDelay    = 15 * time.Minute
	receiptCheckInterval = 5 * time.Minute
	receiptMaxAge        = 2 * time.Hour // give up; Expo drops receipts after 24h anyway
	maxPendingReceipts   = 50000
)

// ─── Wire types ──────────────────────────────────────────────────────────────

type expoMessage struct {
	To        string         `json:"to"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data,omitempty"`
	Sound     string         `json:"sound,omitempty"`     // "default" or omitted for silent
	Badge     *int           `json:"badge,omitempty"`     // iOS only
	ChannelID string         `json:"channelId,omitempty"` // Android only
	Priority  string         `json:"priority,omitempty"`  // "default" | "normal" | "high"
	TTL       int            `json:"ttl,omitempty"`       // seconds
}

type expoTicket struct {
	Status  string `json:"status"` // "ok" | "error"
	ID      string `json:"id"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"` // "DeviceNotRegistered" | "MessageTooBig" | ...
	} `json:"details"`
}

type expoSendResponse struct {
	Data   []expoTicket `json:"data"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

type expoReceipt struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoReceiptsResponse struct {
	Data map[string]expoReceipt `json:"data"`
}

// ─── Service ─────────────────────────────────────────────────────────────────

type pushJob struct {
	UserID    string
	Type      models.NotificationType
	Title     string
	Body      string
	Data      json.RawMessage
	CreatedAt time.Time
}

type pendingReceipt struct {
	Token  string
	SentAt time.Time
}

type PushService struct {
	pool        *pgxpool.Pool
	client      *http.Client
	accessToken string

	queue chan pushJob

	mu       sync.Mutex
	pending  map[string]pendingReceipt // Expo ticket id → token that produced it
	dropped  int64                     // jobs discarded because the queue was full
	sentOK   int64
	sentFail int64
}

// NewPushService constructs the service. It does NOT start the background
// goroutines — call Start for that, so main.go controls the lifecycle and the
// shutdown context.
func NewPushService(pool *pgxpool.Pool, cfg *config.Config) *PushService {
	return &PushService{
		pool: pool,
		client: &http.Client{
			// Generous: Expo occasionally takes several seconds for a full
			// 100-message batch. Still bounded so a hung connection can't wedge
			// the sender loop forever.
			Timeout: 30 * time.Second,
		},
		accessToken: cfg.ExpoAccessToken,
		queue:       make(chan pushJob, queueSize),
		pending:     make(map[string]pendingReceipt),
	}
}

// Start launches the sender and receipt-checker goroutines. Both exit on ctx
// cancellation; the sender drains whatever is already queued first so an
// in-flight fan-out isn't lost to a rolling deploy.
func (s *PushService) Start(ctx context.Context) {
	go s.senderLoop(ctx)
	go s.receiptLoop(ctx)
	log.Info().Bool("expo_access_token", s.accessToken != "").Msg("push service: started")
}

// Dispatch matches repository.PushDispatchFunc.
//
// Non-blocking by contract: it enqueues and returns. When the queue is full it
// DROPS the job and logs, rather than blocking a worker's fan-out loop. Dropping
// is the right trade — the notification row is already committed, so the user
// sees it in-app; the only loss is the buzz.
func (s *PushService) Dispatch(
	userID string,
	notifType models.NotificationType,
	title string,
	body string,
	data json.RawMessage,
) {
	select {
	case s.queue <- pushJob{
		UserID:    userID,
		Type:      notifType,
		Title:     title,
		Body:      body,
		Data:      data,
		CreatedAt: time.Now(),
	}:
	default:
		s.mu.Lock()
		s.dropped++
		total := s.dropped
		s.mu.Unlock()
		log.Warn().
			Str("user_id", userID).
			Str("type", string(notifType)).
			Int64("dropped_total", total).
			Msg("push service: queue full — push dropped (in-app notification still delivered)")
	}
}

// ─── Sender ──────────────────────────────────────────────────────────────────

func (s *PushService) senderLoop(ctx context.Context) {
	timer := time.NewTimer(flushInterval)
	timer.Stop()

	batch := make([]pushJob, 0, maxBatchJobs)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Own context: the batch must still send during graceful shutdown,
		// after the parent ctx has been cancelled.
		sendCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		s.sendBatch(sendCtx, batch)
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case job := <-s.queue:
			if len(batch) == 0 {
				timer.Reset(flushInterval)
			}
			batch = append(batch, job)
			if len(batch) >= maxBatchJobs {
				timer.Stop()
				flush()
			}

		case <-timer.C:
			flush()

		case <-ctx.Done():
			// Drain anything already queued, then stop. Bounded by the queue
			// length so shutdown can't hang on a producer that keeps writing.
			timer.Stop()
			for {
				select {
				case job := <-s.queue:
					batch = append(batch, job)
					if len(batch) >= maxBatchJobs {
						flush()
					}
					continue
				default:
				}
				break
			}
			flush()
			log.Info().Msg("push service: sender stopped")
			return
		}
	}
}

// sendBatch turns a batch of notification jobs into Expo messages and posts
// them. One job becomes zero messages (user has no devices) or several (user
// has several devices).
func (s *PushService) sendBatch(ctx context.Context, batch []pushJob) {
	// 1. Unique recipients.
	userIDSet := make(map[string]struct{}, len(batch))
	for _, j := range batch {
		userIDSet[j.UserID] = struct{}{}
	}
	userIDs := make([]string, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}

	// 2. All devices for all recipients — ONE query.
	targets, err := repository.GetPushTargets(s.pool, ctx, userIDs)
	if err != nil {
		log.Error().Err(err).Int("jobs", len(batch)).Msg("push service: token lookup failed — batch dropped")
		return
	}
	if len(targets) == 0 {
		// Entirely normal: web-only users, or users who never granted
		// permission. Not an error.
		return
	}

	byUser := make(map[string][]models.UserPushTarget, len(userIDs))
	for _, t := range targets {
		byUser[t.UserID] = append(byUser[t.UserID], t)
	}

	// 3. Badge counts — ONE query. Best-effort: a failure here costs the iOS
	// badge number, not the notification.
	badges, err := repository.GetUnreadCounts(s.pool, ctx, userIDs)
	if err != nil {
		log.Warn().Err(err).Msg("push service: unread count lookup failed — sending without badges")
		badges = nil
	}

	// 4. Build messages.
	messages := make([]expoMessage, 0, len(batch))
	tokens := make([]string, 0, len(batch)) // parallel to messages, for ticket→token mapping

	for _, job := range batch {
		devices := byUser[job.UserID]
		if len(devices) == 0 {
			continue
		}

		payload := map[string]any{
			// The client routes the tap on `type` plus whatever ids the
			// producer put in Data (event_id, booking_id, booking_number…).
			"type": string(job.Type),
		}
		if len(job.Data) > 0 {
			var extra map[string]any
			if err := json.Unmarshal(job.Data, &extra); err == nil {
				for k, v := range extra {
					if k == "type" {
						continue // never let payload data shadow the routing key
					}
					payload[k] = v
				}
			}
		}

		var badge *int
		if badges != nil {
			if n, ok := badges[job.UserID]; ok {
				b := n
				badge = &b
			}
		}

		for _, d := range devices {
			msg := expoMessage{
				To:        d.Token,
				Title:     job.Title,
				Body:      job.Body,
				Data:      payload,
				Sound:     "default",
				ChannelID: models.PushChannel(job.Type),
				Priority:  models.PushPriority(job.Type),
				// 1 day. A "your event is tomorrow" reminder that surfaces
				// three days late is noise; better that the OS drops it and
				// the user finds it in the in-app feed.
				TTL: 86400,
			}
			if d.Platform == models.PushPlatformIOS {
				msg.Badge = badge
			}
			messages = append(messages, msg)
			tokens = append(tokens, d.Token)
		}
	}

	if len(messages) == 0 {
		return
	}

	// 5. Post in chunks of 100.
	for start := 0; start < len(messages); start += maxMessagesPerRequest {
		end := start + maxMessagesPerRequest
		if end > len(messages) {
			end = len(messages)
		}
		s.postChunk(ctx, messages[start:end], tokens[start:end])
	}
}

// postChunk sends one ≤100-message request and processes the tickets.
func (s *PushService) postChunk(ctx context.Context, messages []expoMessage, tokens []string) {
	resp, err := s.postWithRetry(ctx, expoSendURL, messages)
	if err != nil {
		log.Error().Err(err).Int("messages", len(messages)).Msg("push service: send failed")
		_ = repository.IncrementPushFailures(s.pool, ctx, tokens)
		return
	}

	var parsed expoSendResponse
	if err := json.Unmarshal(resp, &parsed); err != nil {
		log.Error().Err(err).Str("body", truncateStr(string(resp), 400)).Msg("push service: unparsable send response")
		return
	}

	// Top-level errors mean the whole request was rejected (bad credentials,
	// malformed body) — no per-message tickets come back.
	if len(parsed.Errors) > 0 {
		for _, e := range parsed.Errors {
			log.Error().
				Str("code", e.Code).
				Str("message", e.Message).
				Msg("push service: expo rejected the request")
		}
		return
	}

	// Tickets come back in the same order as the messages sent. If Expo ever
	// returns a different count, the index mapping is unsafe — bail rather than
	// delete the wrong user's token.
	if len(parsed.Data) != len(messages) {
		log.Error().
			Int("sent", len(messages)).
			Int("tickets", len(parsed.Data)).
			Msg("push service: ticket count mismatch — skipping ticket processing")
		return
	}

	var (
		dead      []string
		delivered []string
		okCount   int
		errCount  int
	)

	now := time.Now()
	s.mu.Lock()
	for i, ticket := range parsed.Data {
		token := tokens[i]

		if ticket.Status == "ok" {
			okCount++
			delivered = append(delivered, token)
			if ticket.ID != "" && len(s.pending) < maxPendingReceipts {
				s.pending[ticket.ID] = pendingReceipt{Token: token, SentAt: now}
			}
			continue
		}

		errCount++
		switch ticket.Details.Error {
		case "DeviceNotRegistered":
			// App uninstalled, or the token rotated. Definitively dead.
			dead = append(dead, token)
		case "MessageTooBig", "InvalidCredentials", "MessageRateExceeded":
			log.Warn().
				Str("expo_error", ticket.Details.Error).
				Str("message", ticket.Message).
				Msg("push service: expo ticket error")
		default:
			log.Warn().
				Str("message", ticket.Message).
				Msg("push service: unrecognised expo ticket error")
		}
	}
	s.sentOK += int64(okCount)
	s.sentFail += int64(errCount)
	s.mu.Unlock()

	if len(delivered) > 0 {
		if err := repository.MarkPushTokensUsed(s.pool, ctx, delivered); err != nil {
			log.Warn().Err(err).Msg("push service: failed to stamp last_used_at")
		}
	}
	if len(dead) > 0 {
		n, err := repository.DeletePushTokensByValue(s.pool, ctx, dead)
		if err != nil {
			log.Error().Err(err).Msg("push service: failed to prune dead tokens")
		} else {
			log.Info().Int64("pruned", n).Msg("push service: pruned unregistered devices")
		}
	}

	log.Debug().
		Int("ok", okCount).
		Int("errors", errCount).
		Msg("push service: chunk sent")
}

// ─── Receipts ────────────────────────────────────────────────────────────────
//
// A ticket only means Expo ACCEPTED the message. The receipt, available a few
// minutes later, says whether FCM/APNs actually took it — and it is where most
// DeviceNotRegistered results actually surface. Skipping receipts is how push
// implementations accumulate thousands of dead tokens.
//
// The pending map is IN-MEMORY, so receipts outstanding at shutdown are lost.
// That is an accepted trade: the only consequence is a dead token surviving
// until its next send attempt returns DeviceNotRegistered in the ticket. Making
// it durable would mean another table and another worker for no user-visible
// gain.

func (s *PushService) receiptLoop(ctx context.Context) {
	ticker := time.NewTicker(receiptCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkReceipts(ctx)
		case <-ctx.Done():
			log.Info().Msg("push service: receipt checker stopped")
			return
		}
	}
}

func (s *PushService) checkReceipts(ctx context.Context) {
	now := time.Now()

	s.mu.Lock()
	ids := make([]string, 0, maxReceiptsPerRequest)
	for id, p := range s.pending {
		age := now.Sub(p.SentAt)
		if age > receiptMaxAge {
			delete(s.pending, id) // Expo never produced a receipt; give up.
			continue
		}
		if age < receiptCheckDelay {
			continue // too soon — Expo returns nothing for these
		}
		ids = append(ids, id)
		if len(ids) >= maxReceiptsPerRequest {
			break
		}
	}
	s.mu.Unlock()

	if len(ids) == 0 {
		return
	}

	body, err := s.postWithRetry(ctx, expoReceiptsURL, map[string]any{"ids": ids})
	if err != nil {
		log.Warn().Err(err).Int("ids", len(ids)).Msg("push service: receipt fetch failed")
		return
	}

	var parsed expoReceiptsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Warn().Err(err).Msg("push service: unparsable receipt response")
		return
	}

	var dead []string

	s.mu.Lock()
	for id, receipt := range parsed.Data {
		p, ok := s.pending[id]
		if !ok {
			continue
		}
		delete(s.pending, id)

		if receipt.Status == "error" && receipt.Details.Error == "DeviceNotRegistered" {
			dead = append(dead, p.Token)
		} else if receipt.Status == "error" {
			log.Warn().
				Str("expo_error", receipt.Details.Error).
				Str("message", receipt.Message).
				Msg("push service: delivery receipt error")
		}
	}
	s.mu.Unlock()

	if len(dead) > 0 {
		n, err := repository.DeletePushTokensByValue(s.pool, ctx, dead)
		if err != nil {
			log.Error().Err(err).Msg("push service: failed to prune tokens from receipts")
		} else {
			log.Info().Int64("pruned", n).Msg("push service: pruned unregistered devices (receipts)")
		}
	}
}

// ─── HTTP ────────────────────────────────────────────────────────────────────

// postWithRetry posts JSON to Expo, retrying 429s and 5xx with linear backoff.
// Bounded at three attempts: push is time-sensitive, so a message that can't
// get through in a few seconds is better abandoned than delivered late.
func (s *PushService) postWithRetry(ctx context.Context, url string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	const attempts = 3
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		// Only required when the Expo project has "enhanced security for push
		// notifications" switched on. Harmless otherwise, so it's always sent
		// when configured.
		if s.accessToken != "" {
			req.Header.Set("Authorization", "Bearer "+s.accessToken)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			if !sleepCtx(ctx, time.Duration(attempt)*time.Second) {
				return nil, lastErr
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("expo http %d: %s", resp.StatusCode, truncateStr(string(body), 200))
			if !sleepCtx(ctx, time.Duration(attempt)*2*time.Second) {
				return nil, lastErr
			}
		default:
			// 4xx other than 429 won't improve on retry (bad token format,
			// malformed body, bad credentials).
			return nil, fmt.Errorf("expo http %d: %s", resp.StatusCode, truncateStr(string(body), 400))
		}
	}

	return nil, lastErr
}

// sleepCtx sleeps unless ctx is cancelled first. Returns false if cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Stats exposes counters for a health/debug endpoint.
func (s *PushService) Stats() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]int64{
		"sent_ok":          s.sentOK,
		"sent_failed":      s.sentFail,
		"dropped":          s.dropped,
		"queue_depth":      int64(len(s.queue)),
		"pending_receipts": int64(len(s.pending)),
	}
}