package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tiramission/oci-sync/internal/xdg"
)

const (
	CacheFileName = "activity.json"
	MaxActivities = 100
)

type ActivityType string

const (
	ActivityPush   ActivityType = "push"
	ActivityPull   ActivityType = "pull"
	ActivityDelete ActivityType = "delete"
	ActivityLabel  ActivityType = "label"
)

type Activity struct {
	Type      ActivityType `json:"type"`
	Timestamp time.Time    `json:"timestamp"`
	RemoteRef string       `json:"remote_ref"`
	LocalPath string       `json:"local_path,omitempty"`
	Labels    []string     `json:"labels,omitempty"`
	Success   bool         `json:"success"`
	Error     string       `json:"error,omitempty"`
}

type ActivityCache struct {
	Activities []Activity `json:"activities"`
}

var cacheDir string

func InitCache() error {
	dir := filepath.Join(xdg.CacheDir(), "oci-sync")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	cacheDir = dir
	return nil
}

func CacheFilePath() string {
	if cacheDir == "" {
		cacheDir = filepath.Join(xdg.CacheDir(), "oci-sync")
	}
	return filepath.Join(cacheDir, CacheFileName)
}

func Load() (*ActivityCache, error) {
	path := CacheFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ActivityCache{Activities: []Activity{}}, nil
		}
		return nil, fmt.Errorf("read cache: %w", err)
	}

	var cache ActivityCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse cache: %w", err)
	}

	return &cache, nil
}

func Save(cache *ActivityCache) error {
	path := CacheFilePath()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	return nil
}

func AddActivity(activity Activity) error {
	cache, err := Load()
	if err != nil {
		return err
	}

	cache.Activities = append([]Activity{activity}, cache.Activities...)

	if len(cache.Activities) > MaxActivities {
		cache.Activities = cache.Activities[:MaxActivities]
	}

	return Save(cache)
}

func GetRecentActivities(limit int) ([]Activity, error) {
	cache, err := Load()
	if err != nil {
		return nil, err
	}

	if len(cache.Activities) == 0 {
		return []Activity{}, nil
	}

	if limit > 0 && limit < len(cache.Activities) {
		return cache.Activities[:limit], nil
	}

	return cache.Activities, nil
}

func GetAllActivities() ([]Activity, error) {
	return GetRecentActivities(0)
}

func ClearActivities() error {
	cache := &ActivityCache{Activities: []Activity{}}
	return Save(cache)
}

type ActivityStats struct {
	Total  int            `json:"total"`
	ByType map[string]int `json:"by_type"`
	Recent []Activity     `json:"recent"`
}

func GetStats(limit int) (*ActivityStats, error) {
	cache, err := Load()
	if err != nil {
		return nil, err
	}

	stats := &ActivityStats{
		Total:  len(cache.Activities),
		ByType: make(map[string]int),
	}

	for _, a := range cache.Activities {
		stats.ByType[string(a.Type)]++
	}

	recent := cache.Activities
	if limit > 0 && limit < len(recent) {
		recent = recent[:limit]
	}
	stats.Recent = recent

	sort.Slice(stats.Recent, func(i, j int) bool {
		return stats.Recent[i].Timestamp.After(stats.Recent[j].Timestamp)
	})

	return stats, nil
}
