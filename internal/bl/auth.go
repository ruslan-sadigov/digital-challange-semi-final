package bl

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"smartcity/internal/dl"
	"smartcity/internal/domain"
)

// Session is what the signed cookie carries.
type Session struct {
	UserID    int64       `json:"uid"`
	Role      domain.Role `json:"role"`
	SessionID string      `json:"sid"`
	Email     string      `json:"email,omitempty"`
	ExpiresAt int64       `json:"exp"`
}

type AuthService struct {
	users  dl.UserRepository
	secret []byte
	clock  Clock
	ttl    time.Duration
}

func NewAuthService(users dl.UserRepository, secret string, clock Clock) *AuthService {
	return &AuthService{users: users, secret: []byte(secret), clock: clock, ttl: 24 * time.Hour}
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func validateCredentials(email, pw string) error {
	email = strings.TrimSpace(email)
	if len(email) < 3 || !strings.Contains(email, "@") {
		return domain.ErrInvalidInput("a valid email is required")
	}
	if len(pw) < 6 {
		return domain.ErrInvalidInput("password must be at least 6 characters")
	}
	return nil
}

// Register creates a passenger account.
func (s *AuthService) Register(ctx context.Context, email, pw string) (*domain.User, error) {
	return s.createUser(ctx, email, pw, domain.RolePassenger)
}

// CreateOperator creates an operator account (used by seeding and by other operators).
func (s *AuthService) CreateOperator(ctx context.Context, email, pw string) (*domain.User, error) {
	return s.createUser(ctx, email, pw, domain.RoleOperator)
}

func (s *AuthService) createUser(ctx context.Context, email, pw string, role domain.Role) (*domain.User, error) {
	if err := validateCredentials(email, pw); err != nil {
		return nil, err
	}
	hash, err := HashPassword(pw)
	if err != nil {
		return nil, err
	}
	e := strings.ToLower(strings.TrimSpace(email))
	u := &domain.User{Email: &e, PasswordHash: hash, Role: role, CreatedAt: s.clock.Now()}
	if err := s.users.Create(ctx, u); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, fmt.Errorf("%w: email already registered", domain.ErrConflict)
		}
		return nil, err
	}
	return u, nil
}

// EnsureOperator creates the operator if the email is not registered yet.
func (s *AuthService) EnsureOperator(ctx context.Context, email, pw string) error {
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	_, err := s.CreateOperator(ctx, email, pw)
	return err
}

// Login checks credentials for a sign-in portal: the passenger portal accepts passenger accounts,
// the operator portal accepts operator accounts only.
func (s *AuthService) Login(ctx context.Context, email, pw string, portal domain.Role) (*domain.User, error) {
	u, err := s.checkCredentials(ctx, email, pw)
	if err != nil {
		return nil, err
	}
	switch {
	case portal == domain.RoleOperator && u.Role != domain.RoleOperator:
		return nil, fmt.Errorf("%w: this is not an operator account", domain.ErrForbidden)
	case portal != domain.RoleOperator && u.Role == domain.RoleOperator:
		return nil, fmt.Errorf("%w: operators sign in on the operator tab", domain.ErrForbidden)
	}
	return u, nil
}

// RegisterWithConfirmation validates the confirmation field before creating a passenger account.
func (s *AuthService) RegisterWithConfirmation(ctx context.Context, email, pw, confirm string) (*domain.User, error) {
	if pw != confirm {
		return nil, domain.ErrInvalidInput("passwords do not match")
	}
	return s.Register(ctx, email, pw)
}

func (s *AuthService) checkCredentials(ctx context.Context, email, pw string) (*domain.User, error) {
	u, err := s.users.GetByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if !CheckPassword(u.PasswordHash, pw) {
		return nil, domain.ErrUnauthorized
	}
	return u, nil
}

// Guest creates an anonymous user row so guests can report density and appear on the heatmap.
func (s *AuthService) Guest(ctx context.Context) (*domain.User, error) {
	u := &domain.User{Role: domain.RoleGuest, CreatedAt: s.clock.Now()}
	return u, s.users.Create(ctx, u)
}

// NewSessionID returns a random 128-bit hex id.
func NewSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// IssueToken signs a session for the user: base64(json).hex(hmac-sha256).
func (s *AuthService) IssueToken(u *domain.User) (string, Session) {
	sess := Session{UserID: u.ID, Role: u.Role, SessionID: NewSessionID(), ExpiresAt: s.clock.Now().Add(s.ttl).Unix()}
	if u.Email != nil {
		sess.Email = *u.Email
	}
	return s.sign(sess), sess
}

func (s *AuthService) sign(sess Session) string {
	payload, _ := json.Marshal(sess)
	p := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(p))
	return p + "." + hex.EncodeToString(mac.Sum(nil))
}

// ParseToken verifies the signature and expiry.
func (s *AuthService) ParseToken(token string) (Session, error) {
	p, sig, ok := strings.Cut(token, ".")
	if !ok {
		return Session{}, domain.ErrUnauthorized
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(p))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return Session{}, domain.ErrUnauthorized
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return Session{}, domain.ErrUnauthorized
	}
	var sess Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return Session{}, domain.ErrUnauthorized
	}
	if s.clock.Now().Unix() > sess.ExpiresAt {
		return Session{}, domain.ErrUnauthorized
	}
	return sess, nil
}
