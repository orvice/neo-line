package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// settingsDocID is the fixed _id of the singleton settings document.
const settingsDocID = "site"

// Default site presentation, used when no settings document exists yet.
const (
	DefaultSiteName        = "neo-line"
	DefaultStatusPageTitle = "服务状态"
)

// Settings holds site-wide presentation configuration for the status page.
// It is stored as a single document; MongoDB remains the source of truth.
type Settings struct {
	SiteName        string    `bson:"site_name" json:"site_name"`
	StatusPageTitle string    `bson:"status_page_title" json:"status_page_title"`
	UpdatedAt       time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// GetSettings returns the singleton settings document, falling back to defaults
// when none has been saved yet.
func (s *MongoStore) GetSettings(ctx context.Context) (Settings, error) {
	var out Settings
	err := s.settingsColl().FindOne(ctx, bson.M{"_id": settingsDocID}).Decode(&out)
	if err != nil {
		if IsNotFound(err) {
			return Settings{SiteName: DefaultSiteName, StatusPageTitle: DefaultStatusPageTitle}, nil
		}
		return Settings{}, err
	}
	if out.SiteName == "" {
		out.SiteName = DefaultSiteName
	}
	if out.StatusPageTitle == "" {
		out.StatusPageTitle = DefaultStatusPageTitle
	}
	return out, nil
}

// UpdateSettings upserts the singleton settings document and returns the stored
// value. Empty fields fall back to the defaults so the status page always has
// something to render.
func (s *MongoStore) UpdateSettings(ctx context.Context, settings Settings) (Settings, error) {
	if settings.SiteName == "" {
		settings.SiteName = DefaultSiteName
	}
	if settings.StatusPageTitle == "" {
		settings.StatusPageTitle = DefaultStatusPageTitle
	}
	settings.UpdatedAt = time.Now().UTC()
	_, err := s.settingsColl().UpdateOne(
		ctx,
		bson.M{"_id": settingsDocID},
		bson.M{"$set": settings},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return Settings{}, err
	}
	return settings, nil
}
