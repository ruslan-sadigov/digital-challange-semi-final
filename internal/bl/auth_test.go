package bl

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"smartcity/internal/domain"
)

// memUsers is an in-memory dl.UserRepository for unit tests.
type memUsers struct {
	mu   sync.Mutex
	rows []domain.User
}

func (m *memUsers) Create(_ context.Context, u *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if u.Email != nil && r.Email != nil && strings.EqualFold(*r.Email, *u.Email) {
			return domain.ErrConflict
		}
	}
	u.ID = int64(len(m.rows) + 1)
	m.rows = append(m.rows, *u)
	return nil
}

func (m *memUsers) GetByID(_ context.Context, id int64) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *memUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.Email != nil && strings.EqualFold(*r.Email, email) {
			return &r, nil
		}
	}
	return nil, domain.ErrNotFound
}

func TestPasswordHashing(t *testing.T) {
	h, err := HashPassword("secret1")
	if err != nil {
		t.Fatal(err)
	}
	if h == "secret1" || !CheckPassword(h, "secret1") || CheckPassword(h, "wrong") || CheckPassword("", "") {
		t.Fatal("bcrypt check failed")
	}
}

func TestRegisterLoginGuest(t *testing.T) {
	ctx := context.Background()
	clock := NewFakeClock(t0)
	a := NewAuthService(&memUsers{}, "test-secret", clock)

	if _, err := a.Register(ctx, "bad-email", "secret1"); err == nil {
		t.Fatal("invalid email must fail")
	}
	if _, err := a.Register(ctx, "a@b.az", "123"); err == nil {
		t.Fatal("short password must fail")
	}
	u, err := a.Register(ctx, "Rider@SmartCity.az", "secret1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != domain.RolePassenger || *u.Email != "rider@smartcity.az" {
		t.Fatalf("unexpected user %+v", u)
	}
	if _, err := a.Register(ctx, "rider@smartcity.az", "secret1"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate email: want conflict, got %v", err)
	}
	if _, err := a.Login(ctx, "rider@smartcity.az", "nope00", domain.RolePassenger); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password: got %v", err)
	}
	if _, err := a.Login(ctx, "ghost@smartcity.az", "secret1", domain.RolePassenger); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("unknown email: got %v", err)
	}
	if _, err := a.Login(ctx, "RIDER@smartcity.az", "secret1", domain.RolePassenger); err != nil {
		t.Fatalf("login: %v", err)
	}
	g, err := a.Guest(ctx)
	if err != nil || g.Role != domain.RoleGuest || g.Email != nil {
		t.Fatalf("guest: %+v %v", g, err)
	}
	if err := a.EnsureOperator(ctx, "op@smartcity.az", "operator1"); err != nil {
		t.Fatal(err)
	}
	if err := a.EnsureOperator(ctx, "op@smartcity.az", "operator1"); err != nil {
		t.Fatal("EnsureOperator must be idempotent")
	}
	op, err := a.Login(ctx, "op@smartcity.az", "operator1", domain.RoleOperator)
	if err != nil || op.Role != domain.RoleOperator {
		t.Fatalf("operator portal: %v", err)
	}
	// Portals are separate: operators cannot use the passenger sign-in and vice versa.
	if _, err := a.Login(ctx, "op@smartcity.az", "operator1", domain.RolePassenger); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("operator on passenger portal: %v", err)
	}
	if _, err := a.Login(ctx, "rider@smartcity.az", "secret1", domain.RoleOperator); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("passenger on operator portal: %v", err)
	}
	// Wrong password on the operator portal is still a plain 401, not a hint that the account exists.
	if _, err := a.Login(ctx, "rider@smartcity.az", "wrong00", domain.RoleOperator); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password on operator portal: %v", err)
	}
	if _, err := a.RegisterWithConfirmation(ctx, "new@smartcity.az", "secret1", "secret2"); err == nil {
		t.Fatal("mismatched confirmation must fail")
	}
	if _, err := a.RegisterWithConfirmation(ctx, "new@smartcity.az", "secret1", "secret1"); err != nil {
		t.Fatal(err)
	}
}

func TestTokens(t *testing.T) {
	clock := NewFakeClock(t0)
	a := NewAuthService(&memUsers{}, "test-secret", clock)
	email := "x@y.az"
	token, sess := a.IssueToken(&domain.User{ID: 7, Role: domain.RoleOperator, Email: &email})
	got, err := a.ParseToken(token)
	if err != nil || got.UserID != 7 || got.Role != domain.RoleOperator || got.SessionID != sess.SessionID || len(got.SessionID) != 32 {
		t.Fatalf("parse: %+v %v", got, err)
	}
	// tampered payload
	if _, err := a.ParseToken("x" + token); err == nil {
		t.Fatal("tampered token accepted")
	}
	// signed with another secret
	other := NewAuthService(&memUsers{}, "other", clock)
	if _, err := other.ParseToken(token); err == nil {
		t.Fatal("token from another secret accepted")
	}
	if _, err := a.ParseToken("garbage"); err == nil {
		t.Fatal("garbage accepted")
	}
	clock.Advance(25 * time.Hour)
	if _, err := a.ParseToken(token); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestParseDensity(t *testing.T) {
	for _, s := range []string{"low", "medium", "high"} {
		if _, err := domain.ParseDensity(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"", "unknown", "HIGH", "full"} {
		if _, err := domain.ParseDensity(s); err == nil {
			t.Fatalf("%q should be invalid", s)
		}
	}
}
