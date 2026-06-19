package handlers_test

// =============================================================================
// AD ACCOUNT HANDLER TESTS  (pre-paid wallet model)
// =============================================================================
//
// Tests covered:
//   - POST   /organiser/ad-accounts            -> CreateAdAccountHandler
//   - GET    /organiser/ad-accounts            -> GetAdAccountsHandler
//   - GET    /organiser/ad-accounts/:id        -> GetAdAccountHandler
//   - PATCH  /organiser/ad-accounts/:id        -> UpdateAdAccountHandler
//   - GET    /organiser/ad-wallet              -> GetWalletBalanceHandler
//   - POST   /organiser/events/:id/campaigns   -> CreateCampaignHandler
//
// Ad accounts no longer hold a card / mandate — campaigns are funded by the
// per-organiser ad wallet. A campaign is created 'active' immediately but only
// serves while the wallet balance is positive, so creation no longer requires a
// payment method.
//
// Prerequisites -- start the test database once before running:
//
//	docker run -d --name test-postgres \
//	  -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//	  -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//
// Run:
//
//	go test ./internal/handlers/... -v -run TestAdAccount
//
// =============================================================================

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"events-db/internal/handlers"
	"events-db/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// -- Schema setup -------------------------------------------------------------

func setupAdAccountTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), authTestDBURL)
	if err != nil {
		t.Skipf("ad-account DB tests skipped -- could not create pool: %v", err)
	}
	if pingErr := pool.Ping(context.Background()); pingErr != nil {
		pool.Close()
		t.Skipf("ad-account DB tests skipped -- DB not reachable: %v", pingErr)
	}
	createAdAccountTables(t, pool)
	cleanAdAccountTables(t, pool)
	return pool
}

func createAdAccountTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	drops := []string{
		"DROP TABLE IF EXISTS ad_wallet_topups CASCADE",
		"DROP TABLE IF EXISTS ad_wallets       CASCADE",
		"DROP TABLE IF EXISTS ad_daily_spend   CASCADE",
		"DROP TABLE IF EXISTS ad_campaigns      CASCADE",
		"DROP TABLE IF EXISTS ad_accounts       CASCADE",
		"DROP TABLE IF EXISTS bookings           CASCADE",
		"DROP TABLE IF EXISTS events             CASCADE",
		"DROP TABLE IF EXISTS categories         CASCADE",
		"DROP TABLE IF EXISTS organizers         CASCADE",
		"DROP TABLE IF EXISTS refresh_tokens     CASCADE",
		"DROP TABLE IF EXISTS users              CASCADE",
		"DROP TYPE  IF EXISTS campaign_status",
		"DROP TYPE  IF EXISTS ad_account_status",
		"DROP TYPE  IF EXISTS topup_status",
		"DROP SEQUENCE IF EXISTS ad_topup_ref_seq",
	}
	for _, s := range drops {
		pool.Exec(ctx, s)
	}

	stmts := []string{
		`DO $$ BEGIN
			CREATE TYPE campaign_status AS ENUM
				('active','paused','exhausted','ended','payment_failed');
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`DO $$ BEGIN
			CREATE TYPE ad_account_status AS ENUM ('active','disabled');
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`DO $$ BEGIN
			CREATE TYPE topup_status AS ENUM ('pending','completed','failed');
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,

		`CREATE SEQUENCE IF NOT EXISTS ad_topup_ref_seq START 1`,

		`CREATE TABLE users (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email       VARCHAR(255) UNIQUE NOT NULL,
			password    TEXT NOT NULL DEFAULT '',
			first_name  VARCHAR(100),
			last_name   VARCHAR(100),
			role        VARCHAR(50) NOT NULL DEFAULT 'attendee',
			created_at  TIMESTAMPTZ DEFAULT NOW(),
			updated_at  TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE refresh_tokens (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash  TEXT UNIQUE NOT NULL,
			expires_at  TIMESTAMPTZ NOT NULL,
			revoked     BOOLEAN NOT NULL DEFAULT FALSE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE organizers (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id         UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			business_name   VARCHAR(255) NOT NULL DEFAULT '',
			business_email  VARCHAR(255) NOT NULL DEFAULT '',
			business_phone  VARCHAR(50)  NOT NULL DEFAULT '',
			website_url     TEXT,
			description     TEXT,
			logo_url        TEXT,
			verified        BOOLEAN NOT NULL DEFAULT FALSE,
			status          VARCHAR(50) NOT NULL DEFAULT 'active',
			commission_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
			created_at      TIMESTAMPTZ DEFAULT NOW(),
			updated_at      TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE categories (
			id   SERIAL PRIMARY KEY,
			name VARCHAR(100) UNIQUE NOT NULL,
			slug VARCHAR(100) UNIQUE NOT NULL
		)`,

		`CREATE TABLE events (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title          VARCHAR(255) NOT NULL,
			description    TEXT NOT NULL DEFAULT '',
			start_time     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
			end_time       TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '8 days',
			category_id    INTEGER NOT NULL REFERENCES categories(id),
			price          DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			currency       VARCHAR(3)  NOT NULL DEFAULT 'ZAR',
			max_capacity   INTEGER,
			tickets_sold   INTEGER NOT NULL DEFAULT 0,
			status         VARCHAR(50) NOT NULL DEFAULT 'published',
			is_featured    BOOLEAN DEFAULT FALSE,
			organizer_id   UUID REFERENCES users(id),
			views          BIGINT DEFAULT 0,
			created_at     TIMESTAMPTZ DEFAULT NOW(),
			updated_at     TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Ad accounts no longer store card / mandate columns.
		`CREATE TABLE ad_accounts (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name         TEXT NOT NULL CHECK (char_length(name) BETWEEN 2 AND 100),
			status       ad_account_status NOT NULL DEFAULT 'active',
			currency     CHAR(3) NOT NULL DEFAULT 'ZAR',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE TABLE ad_campaigns (
			id                UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id          UUID            NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			organizer_id      UUID            NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
			ad_account_id     UUID            REFERENCES ad_accounts(id) ON DELETE SET NULL,
			daily_budget      DECIMAL(10,2)   NOT NULL,
			cpm_rate          DECIMAL(8,4)    NOT NULL DEFAULT 20.00,
			total_spent       DECIMAL(10,2)   NOT NULL DEFAULT 0.00,
			total_impressions BIGINT          NOT NULL DEFAULT 0,
			total_clicks      BIGINT          NOT NULL DEFAULT 0,
			start_date        DATE            NOT NULL DEFAULT CURRENT_DATE,
			end_date          DATE,
			status            campaign_status NOT NULL DEFAULT 'active',
			created_at        TIMESTAMPTZ DEFAULT NOW(),
			updated_at        TIMESTAMPTZ DEFAULT NOW(),
			CONSTRAINT chk_campaign_daily_budget_min CHECK (daily_budget >= 200),
			CONSTRAINT chk_campaign_end_after_start  CHECK (end_date IS NULL OR end_date >= start_date)
		)`,

		`CREATE TABLE ad_daily_spend (
			id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
			campaign_id  UUID          NOT NULL REFERENCES ad_campaigns(id) ON DELETE CASCADE,
			spend_date   DATE          NOT NULL DEFAULT CURRENT_DATE,
			impressions  BIGINT        NOT NULL DEFAULT 0,
			clicks       BIGINT        NOT NULL DEFAULT 0,
			amount_spent DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			UNIQUE (campaign_id, spend_date)
		)`,

		// Per-organiser pre-paid wallet (keyed by users.id).
		`CREATE TABLE ad_wallets (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			balance      DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			total_topups DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			total_spent  DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			created_at   TIMESTAMPTZ DEFAULT NOW(),
			updated_at   TIMESTAMPTZ DEFAULT NOW(),
			CONSTRAINT chk_wallet_balance_non_negative CHECK (balance >= 0)
		)`,

		`CREATE TABLE ad_wallet_topups (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			amount             DECIMAL(10,2) NOT NULL,
			paystack_reference VARCHAR(255) UNIQUE NOT NULL,
			reference_number   VARCHAR(20) UNIQUE,
			status             topup_status NOT NULL DEFAULT 'pending',
			created_at         TIMESTAMPTZ DEFAULT NOW(),
			completed_at       TIMESTAMPTZ
		)`,
	}

	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("createAdAccountTables: %v\nSQL: %s", err, s)
		}
	}
}

func cleanAdAccountTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "TRUNCATE ad_wallet_topups CASCADE")
	pool.Exec(ctx, "TRUNCATE ad_wallets       CASCADE")
	pool.Exec(ctx, "TRUNCATE ad_daily_spend   CASCADE")
	pool.Exec(ctx, "TRUNCATE ad_campaigns     CASCADE")
	pool.Exec(ctx, "TRUNCATE ad_accounts      CASCADE")
	pool.Exec(ctx, "TRUNCATE events           CASCADE")
	pool.Exec(ctx, "TRUNCATE categories       CASCADE")
	pool.Exec(ctx, "TRUNCATE organizers       CASCADE")
	pool.Exec(ctx, "TRUNCATE refresh_tokens   CASCADE")
	pool.Exec(ctx, "TRUNCATE users            CASCADE")
}

// -- Fixtures -----------------------------------------------------------------

func seedOrganizerUser(t *testing.T, pool *pgxpool.Pool) (orgID, token string) {
	t.Helper()
	ctx := context.Background()
	cfg := authTestConfig()

	email := randomEmail()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Password1234!"), bcrypt.DefaultCost)
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password, role)
		VALUES ($1, $2, 'organizer')
		RETURNING id
	`, email, string(hashed)).Scan(&orgID)
	require.NoError(t, err)

	pool.Exec(ctx, `
		INSERT INTO organizers (id, user_id, business_name)
		VALUES ($1, $1, 'Test Org')
		ON CONFLICT (id) DO NOTHING
	`, orgID)

	r := buildAuthRouter(pool, cfg)
	w := doRequest(r, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": "Password1234!",
	})
	require.Equal(t, http.StatusOK, w.Code, "seedOrganizerUser login failed: %s", w.Body.String())
	var authResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &authResp))
	token = authResp["access_token"].(string)
	return orgID, token
}

func seedAdAccountFixtures(t *testing.T, pool *pgxpool.Pool) (orgID, eventID, token string) {
	t.Helper()
	ctx := context.Background()

	orgID, token = seedOrganizerUser(t, pool)

	var catID int
	err := pool.QueryRow(ctx, `
		INSERT INTO categories (name, slug)
		VALUES ('Music','music')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`).Scan(&catID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		INSERT INTO events (title, description, category_id, organizer_id, status)
		VALUES ('Test Concert','A great show', $1, $2, 'published')
		RETURNING id
	`, catID, orgID).Scan(&eventID)
	require.NoError(t, err)

	return orgID, eventID, token
}

// creditWallet tops the organiser's wallet up directly (bypassing Paystack) so
// "serving" assertions can be exercised without a live payment.
func creditWallet(t *testing.T, pool *pgxpool.Pool, organizerID string, amount float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO ad_wallets (organizer_id, balance, total_topups)
		VALUES ($1, $2, $2)
		ON CONFLICT (organizer_id) DO UPDATE
			SET balance = ad_wallets.balance + EXCLUDED.balance,
			    total_topups = ad_wallets.total_topups + EXCLUDED.total_topups
	`, organizerID, amount)
	require.NoError(t, err)
}

// -- Router -------------------------------------------------------------------

func buildAdAccountRouter(pool *pgxpool.Pool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := authTestConfig()
	r := gin.New()

	org := r.Group("/organiser")
	org.Use(middleware.AuthMiddleware(cfg))
	{
		org.POST("/ad-accounts", handlers.CreateAdAccountHandler(pool))
		org.GET("/ad-accounts", handlers.GetAdAccountsHandler(pool))
		org.GET("/ad-accounts/:id", handlers.GetAdAccountHandler(pool))
		org.PATCH("/ad-accounts/:id", handlers.UpdateAdAccountHandler(pool))

		org.GET("/ad-wallet", handlers.GetWalletBalanceHandler(pool))

		org.POST("/events/:id/campaigns", handlers.CreateCampaignHandler(pool))
		org.GET("/campaigns", handlers.GetMyCampaignsHandler(pool))
		org.GET("/campaigns/:id", handlers.GetCampaignHandler(pool))
		org.PATCH("/campaigns/:id", handlers.UpdateCampaignHandler(pool))
		org.DELETE("/campaigns/:id", handlers.EndCampaignHandler(pool))
	}

	auth := r.Group("/auth")
	auth.POST("/login", handlers.LoginHandler(pool, cfg))
	auth.POST("/register", handlers.CreateUserhandler(pool))
	auth.POST("/refresh", handlers.RefreshTokenHandler(pool, cfg))

	return r
}

// -- Helpers ------------------------------------------------------------------

func createAccountAndGetID(t *testing.T, r *gin.Engine, token, name string) string {
	t.Helper()
	w := doAuthRequest(r, http.MethodPost, "/organiser/ad-accounts",
		map[string]interface{}{"name": name}, token)
	require.Equal(t, http.StatusCreated, w.Code, "createAccount failed: %s", w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp["account"].(map[string]interface{})["id"].(string)
}

// -- Tests: Create ------------------------------------------------------------

func TestAdAccount_Create_OK(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	w := doAuthRequest(r, http.MethodPost, "/organiser/ad-accounts",
		map[string]interface{}{"name": "Thetsongaboys Main"}, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	account := resp["account"].(map[string]interface{})
	assert.NotEmpty(t, account["id"])
	assert.Equal(t, "Thetsongaboys Main", account["name"])
	assert.Equal(t, "active", account["status"])
	assert.Equal(t, false, account["has_payment_method"], "ad accounts no longer carry a payment method")
	assert.NotEmpty(t, resp["next"], "response should include next-step hint (top up the wallet)")
}

func TestAdAccount_Create_Unauthenticated(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	r := buildAdAccountRouter(pool)
	w := doRequest(r, http.MethodPost, "/organiser/ad-accounts",
		map[string]interface{}{"name": "No Auth"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdAccount_Create_NameTooShort(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	w := doAuthRequest(r, http.MethodPost, "/organiser/ad-accounts",
		map[string]interface{}{"name": "X"}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestAdAccount_Create_MissingName(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	w := doAuthRequest(r, http.MethodPost, "/organiser/ad-accounts",
		map[string]interface{}{}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestAdAccount_Create_MultipleAccountsAllowed(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	createAccountAndGetID(t, r, token, "Brand A")
	createAccountAndGetID(t, r, token, "Brand B")

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts", nil, token)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["count"])
}

// -- Tests: Get list ----------------------------------------------------------

func TestAdAccount_GetList_Empty(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts", nil, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
	assert.Empty(t, resp["accounts"])
}

func TestAdAccount_GetList_ReturnsDenormalisedStats(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	createAccountAndGetID(t, r, token, "Stats Account")

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts", nil, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(1), resp["count"])

	account := resp["accounts"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, float64(0), account["total_spent"])
	assert.Equal(t, float64(0), account["active_campaigns"])
}

func TestAdAccount_GetList_IsolatedBetweenOrganizers(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, tokenA := seedAdAccountFixtures(t, pool)
	_, tokenB := seedOrganizerUser(t, pool)
	r := buildAdAccountRouter(pool)

	createAccountAndGetID(t, r, tokenA, "Org A Account")

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts", nil, tokenB)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"], "org B should not see org A's accounts")
}

// -- Tests: Get single --------------------------------------------------------

func TestAdAccount_GetSingle_Found(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, token, "My Account")

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts/"+accountID, nil, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var account map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &account))
	assert.Equal(t, accountID, account["id"])
	assert.Equal(t, "My Account", account["name"])
}

func TestAdAccount_GetSingle_NotFound(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	w := doAuthRequest(r, http.MethodGet,
		"/organiser/ad-accounts/00000000-0000-0000-0000-000000000000", nil, token)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdAccount_GetSingle_CrossOrganizerBlocked(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, tokenA := seedAdAccountFixtures(t, pool)
	_, tokenB := seedOrganizerUser(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, tokenA, "Private Account")

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts/"+accountID, nil, tokenB)
	assert.Equal(t, http.StatusNotFound, w.Code,
		"cross-organizer account fetch should return 404 not 403 (avoid leaking existence)")
}

// -- Tests: Update ------------------------------------------------------------

func TestAdAccount_Update_Name(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, token, "Old Name")

	w := doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"name": "New Name"}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "New Name", resp["account"].(map[string]interface{})["name"])
}

func TestAdAccount_Update_Disable(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, token, "Active Account")

	w := doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"status": "disabled"}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "disabled", resp["account"].(map[string]interface{})["status"])
}

func TestAdAccount_Update_ReEnable(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, token, "Account")

	doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"status": "disabled"}, token)

	w := doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"status": "active"}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "active", resp["account"].(map[string]interface{})["status"])
}

func TestAdAccount_Update_CrossOrganizerBlocked(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, tokenA := seedAdAccountFixtures(t, pool)
	_, tokenB := seedOrganizerUser(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, tokenA, "Org A Account")

	w := doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"name": "Hijacked"}, tokenB)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// -- Tests: Wallet ------------------------------------------------------------

func TestAdAccount_Wallet_StartsEmpty(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-wallet", nil, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["balance"])
	assert.Equal(t, false, resp["can_serve"], "an empty wallet cannot serve impressions")
}

func TestAdAccount_Wallet_ReflectsCredit(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	orgID, _, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	creditWallet(t, pool, orgID, 500.00)

	w := doAuthRequest(r, http.MethodGet, "/organiser/ad-wallet", nil, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(500), resp["balance"])
	assert.Equal(t, true, resp["can_serve"])
}

// -- Tests: Campaign creation via ad account ----------------------------------

func TestAdAccount_Campaign_Create_OK(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	orgID, eventID, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	creditWallet(t, pool, orgID, 1000.00)
	accountID := createAccountAndGetID(t, r, token, "Campaign Account")

	w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{
			"ad_account_id": accountID,
			"daily_budget":  200,
		}, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	campaign := resp["campaign"].(map[string]interface{})
	assert.Equal(t, "active", campaign["status"])
	assert.Equal(t, accountID, campaign["ad_account_id"])
	assert.Equal(t, float64(200), campaign["daily_budget"])
	assert.Equal(t, true, resp["serving"], "a funded wallet means the campaign serves immediately")
}

// Without a wallet top-up the campaign is still created (active) but will not
// serve until the organiser funds the wallet.
func TestAdAccount_Campaign_Create_NoFunds_CreatedButNotServing(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, eventID, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, token, "Unfunded Account")

	w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{
			"ad_account_id": accountID,
			"daily_budget":  200,
		}, token)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	campaign := resp["campaign"].(map[string]interface{})
	assert.Equal(t, "active", campaign["status"])
	assert.Equal(t, false, resp["serving"], "no wallet balance → campaign created but not serving")
}

func TestAdAccount_Campaign_Create_DisabledAccount(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	orgID, eventID, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	creditWallet(t, pool, orgID, 1000.00)
	accountID := createAccountAndGetID(t, r, token, "Account")

	doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"status": "disabled"}, token)

	w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{
			"ad_account_id": accountID,
			"daily_budget":  200,
		}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestAdAccount_Campaign_Create_WrongOrganizer(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, eventID, _ := seedAdAccountFixtures(t, pool)
	_, tokenB := seedOrganizerUser(t, pool)
	r := buildAdAccountRouter(pool)

	accountIDofB := createAccountAndGetID(t, r, tokenB, "Org B Account")

	// Org B tries to boost Org A's event using Org B's account.
	w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{
			"ad_account_id": accountIDofB,
			"daily_budget":  200,
		}, tokenB)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestAdAccount_Campaign_Create_CrossAccountBlocked(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, eventID, tokenA := seedAdAccountFixtures(t, pool)
	_, tokenB := seedOrganizerUser(t, pool)
	r := buildAdAccountRouter(pool)

	accountIDofB := createAccountAndGetID(t, r, tokenB, "Org B Account")

	// Org A tries to use Org B's account to boost Org A's own event.
	w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{
			"ad_account_id": accountIDofB,
			"daily_budget":  200,
		}, tokenA)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestAdAccount_Campaign_Create_MissingAdAccountID(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, eventID, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{"daily_budget": 200}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestAdAccount_Campaign_Create_BudgetTooLow(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	_, eventID, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	accountID := createAccountAndGetID(t, r, token, "Account")

	w := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{
			"ad_account_id": accountID,
			"daily_budget":  50,
		}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestAdAccount_Campaign_Create_DuplicateBlocked(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	orgID, eventID, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	creditWallet(t, pool, orgID, 1000.00)
	accountID := createAccountAndGetID(t, r, token, "Account")

	body := map[string]interface{}{"ad_account_id": accountID, "daily_budget": 200}

	w1 := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns", body, token)
	require.Equal(t, http.StatusCreated, w1.Code, "first campaign should succeed")

	w2 := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns", body, token)
	assert.Equal(t, http.StatusConflict, w2.Code, "duplicate campaign should be rejected")
}

// -- Tests: Full flow ---------------------------------------------------------

// TestAdAccount_FullFlow walks the wallet-era lifecycle:
// create account -> wallet empty -> credit wallet -> create campaign (serving) ->
// active_campaigns=1 -> second account -> disable -> re-enable.
func TestAdAccount_FullFlow(t *testing.T) {
	pool := setupAdAccountTestDB(t)
	defer pool.Close()

	orgID, eventID, token := seedAdAccountFixtures(t, pool)
	r := buildAdAccountRouter(pool)

	// 1. Create ad account.
	w1 := doAuthRequest(r, http.MethodPost, "/organiser/ad-accounts",
		map[string]interface{}{"name": "Thetsongaboys Main Account"}, token)
	require.Equal(t, http.StatusCreated, w1.Code, w1.Body.String())
	var createResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &createResp))
	account := createResp["account"].(map[string]interface{})
	accountID := account["id"].(string)
	assert.Equal(t, "active", account["status"])
	assert.Equal(t, false, account["has_payment_method"])

	// 2. Wallet starts empty.
	wWallet1 := doAuthRequest(r, http.MethodGet, "/organiser/ad-wallet", nil, token)
	require.Equal(t, http.StatusOK, wWallet1.Code)
	var walletResp1 map[string]interface{}
	require.NoError(t, json.Unmarshal(wWallet1.Body.Bytes(), &walletResp1))
	assert.Equal(t, float64(0), walletResp1["balance"])
	assert.Equal(t, false, walletResp1["can_serve"])

	// 3. Fund the wallet (stands in for a confirmed Paystack top-up).
	creditWallet(t, pool, orgID, 600.00)

	wWallet2 := doAuthRequest(r, http.MethodGet, "/organiser/ad-wallet", nil, token)
	require.Equal(t, http.StatusOK, wWallet2.Code)
	var walletResp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(wWallet2.Body.Bytes(), &walletResp2))
	assert.Equal(t, float64(600), walletResp2["balance"])
	assert.Equal(t, true, walletResp2["can_serve"])

	// 4. Create a campaign — active and serving since the wallet is funded.
	wCamp := doAuthRequest(r, http.MethodPost, "/organiser/events/"+eventID+"/campaigns",
		map[string]interface{}{"ad_account_id": accountID, "daily_budget": 200}, token)
	require.Equal(t, http.StatusCreated, wCamp.Code, wCamp.Body.String())
	var campResp map[string]interface{}
	require.NoError(t, json.Unmarshal(wCamp.Body.Bytes(), &campResp))
	camp := campResp["campaign"].(map[string]interface{})
	assert.Equal(t, "active", camp["status"])
	assert.Equal(t, accountID, camp["ad_account_id"])
	assert.Equal(t, true, campResp["serving"])

	// 5. Account list shows active_campaigns = 1.
	wList := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts", nil, token)
	require.Equal(t, http.StatusOK, wList.Code)
	var listResp map[string]interface{}
	require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &listResp))
	require.Equal(t, float64(1), listResp["count"])
	listedAccount := listResp["accounts"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, float64(1), listedAccount["active_campaigns"])

	// 6. Second account — list now has 2.
	createAccountAndGetID(t, r, token, "Thetsongaboys Events Account")
	wList2 := doAuthRequest(r, http.MethodGet, "/organiser/ad-accounts", nil, token)
	require.Equal(t, http.StatusOK, wList2.Code)
	var listResp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(wList2.Body.Bytes(), &listResp2))
	assert.Equal(t, float64(2), listResp2["count"])

	// 7. Disable the first account.
	wDisable := doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"status": "disabled"}, token)
	require.Equal(t, http.StatusOK, wDisable.Code, wDisable.Body.String())
	var disableResp map[string]interface{}
	require.NoError(t, json.Unmarshal(wDisable.Body.Bytes(), &disableResp))
	assert.Equal(t, "disabled", disableResp["account"].(map[string]interface{})["status"])

	// 8. Re-enable.
	wEnable := doAuthRequest(r, http.MethodPatch, "/organiser/ad-accounts/"+accountID,
		map[string]interface{}{"status": "active"}, token)
	require.Equal(t, http.StatusOK, wEnable.Code, wEnable.Body.String())
	var enableResp map[string]interface{}
	require.NoError(t, json.Unmarshal(wEnable.Body.Bytes(), &enableResp))
	assert.Equal(t, "active", enableResp["account"].(map[string]interface{})["status"])
}