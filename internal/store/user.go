package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

const (
	RoleAdmin = "admin"

	// SessionTTL is how long an issued login token stays valid.
	SessionTTL = 24 * time.Hour

	sessionKeyPrefix = "neo-line:session:"
)

// ErrInvalidCredentials is returned when an email/password pair does not match.
var ErrInvalidCredentials = errors.New("invalid credentials")

// User is an account that can authenticate against the admin API.
type User struct {
	ID           string    `bson:"id" json:"id"`
	Email        string    `bson:"email" json:"email"`
	PasswordHash string    `bson:"password_hash" json:"-"`
	Role         string    `bson:"role" json:"role"`
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

// Session is an issued bearer token bound to a user.
type Session struct {
	Token     string    `bson:"token" json:"token"`
	UserID    string    `bson:"user_id" json:"user_id"`
	Email     string    `bson:"email" json:"email"`
	Role      string    `bson:"role" json:"role"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"`
}

// EnsureAuthIndexes creates the indexes the user system relies on. The unique
// email index keeps account creation idempotent. Login sessions are stored in
// Redis with per-token TTLs, so no MongoDB session indexes are required.
func (s *MongoStore) EnsureAuthIndexes(ctx context.Context) error {
	_, err := s.users().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

// EnsureAdminUser makes the configured admin account match the supplied
// credentials. The admin password comes from the environment, so it is the
// source of truth: an existing account has its password hash refreshed on
// every startup, allowing rotation by changing the env value.
func (s *MongoStore) EnsureAdminUser(ctx context.Context, email, password string) error {
	email = normalizeEmail(email)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res := s.users().FindOneAndUpdate(
		ctx,
		bson.M{"email": email},
		bson.M{
			"$set": bson.M{
				"password_hash": string(hash),
				"role":          RoleAdmin,
				"updated_at":    now,
			},
			"$setOnInsert": bson.M{
				"id":         "usr_" + uuid.NewString(),
				"email":      email,
				"created_at": now,
			},
		},
		options.FindOneAndUpdate().SetUpsert(true),
	)
	if err := res.Err(); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	return nil
}

// Authenticate verifies an email/password pair and returns the matching user.
func (s *MongoStore) Authenticate(ctx context.Context, email, password string) (User, error) {
	var user User
	err := s.users().FindOne(ctx, bson.M{"email": normalizeEmail(email)}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

// CreateSession issues a bearer token for a user and stores it in Redis.
func (s *MongoStore) CreateSession(ctx context.Context, user User) (Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	session := Session{
		Token:     hex.EncodeToString(tokenBytes),
		UserID:    user.ID,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionTTL),
	}
	data, err := json.Marshal(session)
	if err != nil {
		return Session{}, err
	}
	if err := s.sessionClient.Set(ctx, sessionKey(session.Token), data, SessionTTL).Err(); err != nil {
		return Session{}, err
	}
	return session, nil
}

// GetSession returns a valid session for a token. Redis expires tokens by TTL;
// the ExpiresAt check is retained to reject stale values defensively.
func (s *MongoStore) GetSession(ctx context.Context, token string) (Session, error) {
	data, err := s.sessionClient.Get(ctx, sessionKey(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Session{}, mongo.ErrNoDocuments
	}
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, err
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_, _ = s.sessionClient.Del(ctx, sessionKey(token)).Result()
		return Session{}, mongo.ErrNoDocuments
	}
	return session, nil
}

// DeleteSession revokes a token.
func (s *MongoStore) DeleteSession(ctx context.Context, token string) error {
	return s.sessionClient.Del(ctx, sessionKey(token)).Err()
}

func sessionKey(token string) string {
	return sessionKeyPrefix + token
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *MongoStore) users() *mongo.Collection { return s.database.Collection("users") }
