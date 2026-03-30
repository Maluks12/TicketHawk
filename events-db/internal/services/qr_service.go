package services

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"events-db/internal/config"
	"events-db/internal/models"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/skip2/go-qrcode"
)

type QRService struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewQRService(pool *pgxpool.Pool, cfg *config.Config) *QRService {
	return &QRService{
		pool: pool,
		cfg:  cfg,
	}
}

type QRPayload struct {
	BookingID     string    `json:"booking_id"`
	BookingNumber string    `json:"booking_number"`
	EventID       string    `json:"event_id"`
	UserID        string    `json:"user_id"`
	AssignmentID  string    `json:"assignment_id"`
	SlotNumber    int       `json:"slot_number"`
	Quantity      int       `json:"quantity"`
	IssuedAt      time.Time `json:"issued_at"`
}

func (s *QRService) GenerateQRCode(bookingID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT booking_number, event_id, user_id, ticket_quantity
		FROM bookings
		WHERE id = $1
	`

	var payload QRPayload
	payload.BookingID = bookingID
	payload.IssuedAt = time.Now()

	// user_id is NULL for guest bookings — scan into *string to handle safely
	var userID *string

	err := s.pool.QueryRow(ctx, query, bookingID).Scan(
		&payload.BookingNumber,
		&payload.EventID,
		&userID,
		&payload.Quantity,
	)
	if err != nil {
		return fmt.Errorf("failed to get booking: %w", err)
	}

	// Guests have no user_id — fall back to booking ID as the unique identifier
	if userID != nil {
		payload.UserID = *userID
	} else {
		payload.UserID = bookingID
	}

	encryptedData, err := s.encryptPayload(payload)
	if err != nil {
		return fmt.Errorf("failed to encrypt payload: %w", err)
	}

	png, err := qrcode.Encode(encryptedData, qrcode.Medium, 256)
	if err != nil {
		return fmt.Errorf("failed to generate QR code: %w", err)
	}

	qrURL, err := s.uploadQRCode(bookingID, png)
	if err != nil {
		return fmt.Errorf("failed to upload QR code: %w", err)
	}

	updateQuery := `
		UPDATE bookings
		SET qr_code_data = $1, qr_code_url = $2, tickets_sent = TRUE
		WHERE id = $3
	`
	_, err = s.pool.Exec(ctx, updateQuery, encryptedData, qrURL, bookingID)
	if err != nil {
		return fmt.Errorf("failed to update booking: %w", err)
	}
	return nil
}

func (s *QRService) VerifyQRCode(encryptedData string) (*QRPayload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
 
	payload, err := s.decryptPayload(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("invalid QR code: %w", err)
	}
 
	var bookingID     string
	var bookingStatus string
	var startTime, endTime time.Time
 
	err = s.pool.QueryRow(ctx, `
		SELECT b.id, b.booking_status, e.start_time, e.end_time
		FROM bookings b
		JOIN events e ON b.event_id = e.id
		WHERE b.id = $1
	`, payload.BookingID).Scan(
		&bookingID,
		&bookingStatus,
		&startTime,
		&endTime,
	)
	if err != nil {
		return nil, fmt.Errorf("booking not found: %w", err)
	}
 
	// ── Guard: booking must be confirmed ──────────────────────────────────────
	if bookingStatus != string(models.BookingConfirmed) {
		return nil, fmt.Errorf("ticket is no longer valid — the booking has been %s", bookingStatus)
	}
 
	now := time.Now()
	if now.Before(startTime.Add(-2*time.Hour)) || now.After(endTime.Add(2*time.Hour)) {
		return nil, fmt.Errorf("QR code not valid for this time")
	}
 
	return payload, nil
}

func (s *QRService) encryptPayload(payload QRPayload) (string, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher([]byte(s.cfg.QREncryptionKey))
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, jsonData, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *QRService) decryptPayload(encryptedData string) (*QRPayload, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher([]byte(s.cfg.QREncryptionKey))
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("invalid encrypted data")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var payload QRPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (s *QRService) uploadQRCode(bookingID string, pngData []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				s.cfg.R2AccessKey,
				s.cfg.R2SecretKey,
				"",
			),
		),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s.cfg.R2URL)
	})

	filename := fmt.Sprintf("qr-codes/%s.png", bookingID)

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.R2Bucket),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(pngData),
		ContentType: aws.String("image/png"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload QR code: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", s.cfg.R2URL, s.cfg.R2Bucket, filename), nil
}

// This replaces the booking-level GenerateQRCode for all new confirmed bookings.
func (s *QRService) GenerateQRCodeForAssignment(assignmentID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
 
	query := `
		SELECT
			ta.id, ta.booking_id, ta.slot_number,
			b.booking_number, b.event_id, b.user_id, b.ticket_quantity
		FROM ticket_assignments ta
		JOIN bookings b ON ta.booking_id = b.id
		WHERE ta.id = $1
	`
 
	var (
		payload      QRPayload
		userID       *string
		slotNumber   int
	)
	payload.IssuedAt = time.Now()
 
	err := s.pool.QueryRow(ctx, query, assignmentID).Scan(
		&payload.AssignmentID,
		&payload.BookingID,
		&slotNumber,
		&payload.BookingNumber,
		&payload.EventID,
		&userID,
		&payload.Quantity,
	)
	if err != nil {
		return fmt.Errorf("fetch assignment for QR: %w", err)
	}
 
	payload.SlotNumber = slotNumber
	if userID != nil {
		payload.UserID = *userID
	} else {
		payload.UserID = payload.BookingID
	}
 
	encryptedData, err := s.encryptPayload(payload)
	if err != nil {
		return fmt.Errorf("encrypt payload: %w", err)
	}
 
	png, err := qrcode.Encode(encryptedData, qrcode.Medium, 256)
	if err != nil {
		return fmt.Errorf("generate QR PNG: %w", err)
	}
 
	// Use assignment ID as the object key so each slot has its own file.
	qrURL, err := s.uploadQRCode(assignmentID, png)
	if err != nil {
		return fmt.Errorf("upload QR PNG: %w", err)
	}
 
	_, err = s.pool.Exec(ctx, `
		UPDATE ticket_assignments
		SET qr_code_data = $1, qr_code_url = $2
		WHERE id = $3
	`, encryptedData, qrURL, assignmentID)
 
	return err
}
 
// VerifyAssignmentQRCode decrypts a QR payload and validates it against the
// ticket_assignments table. Used by organisers scanning at the door.
// The returned payload includes AssignmentID so you can mark the slot as used
// if you want single-entry enforcement in a future iteration.
func (s *QRService) VerifyAssignmentQRCode(encryptedData string) (*QRPayload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
 
	payload, err := s.decryptPayload(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("invalid QR code: %w", err)
	}
 
	var assignmentStatus string
	var bookingStatus    string
	var startTime, endTime time.Time
 
	err = s.pool.QueryRow(ctx, `
		SELECT
			ta.status,
			b.booking_status,
			e.start_time,
			e.end_time
		FROM ticket_assignments ta
		JOIN bookings b ON ta.booking_id = b.id
		JOIN events   e ON b.event_id    = e.id
		WHERE ta.id = $1
	`, payload.AssignmentID).Scan(
		&assignmentStatus,
		&bookingStatus,
		&startTime,
		&endTime,
	)
	if err != nil {
		return nil, fmt.Errorf("ticket not found: %w", err)
	}
 
	// ── Guard 1: assignment must be claimed ───────────────────────────────────
	if assignmentStatus != string(models.AssignmentClaimed) {
		return nil, fmt.Errorf("ticket is not valid (status: %s)", assignmentStatus)
	}
 
	// ── Guard 2: parent booking must still be confirmed ───────────────────────
	// This catches refunded and cancelled bookings — the critical fix.
	if bookingStatus != string(models.BookingConfirmed) {
		return nil, fmt.Errorf("ticket is no longer valid — the booking has been %s", bookingStatus)
	}
 
	// ── Guard 3: event time window ────────────────────────────────────────────
	now := time.Now()
	if now.Before(startTime.Add(-2*time.Hour)) || now.After(endTime.Add(2*time.Hour)) {
		return nil, fmt.Errorf("QR code is not valid for this time window")
	}
 
	return payload, nil
}