package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// useTempDir points the package-level cacheDir at a fresh temp directory so
// tests never touch the real user cache.
func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := cacheDir
	cacheDir = dir
	t.Cleanup(func() { cacheDir = orig })
	return dir
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	useTempDir(t)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got == nil || len(got.Activities) != 0 {
		t.Errorf("got %+v, want an empty cache", got)
	}
}

func TestAddActivityPrependsNewestFirst(t *testing.T) {
	useTempDir(t)

	base := time.Now()
	for i, name := range []string{"first", "second", "third"} {
		if err := AddActivity(Activity{
			Type:      ActivityPush,
			Timestamp: base.Add(time.Duration(i) * time.Second),
			RemoteRef: name,
			Success:   true,
		}); err != nil {
			t.Fatalf("AddActivity(%s): %v", name, err)
		}
	}

	got, err := GetAllActivities()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"third", "second", "first"}
	if len(got) != len(want) {
		t.Fatalf("got %d activities, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].RemoteRef != w {
			t.Errorf("activities[%d] = %q, want %q (newest first)", i, got[i].RemoteRef, w)
		}
	}
}

// Save must survive a handle held on the destination for a few
// milliseconds, the way a Windows real-time scanner holds a just-written
// cache file open. rename-over-existing fails while it does, and without a
// retry the activity is dropped from the history even though the push or
// pull that triggered it succeeded. The lock is released mid-retry, so
// Save must come back with nil — on other platforms the first rename
// already succeeds and the lock never matters.
func TestSaveToleratesTransientLockOnDestination(t *testing.T) {
	dir := useTempDir(t)
	path := filepath.Join(dir, CacheFileName)
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// An open handle without share-delete is exactly what a scanner
	// leaves behind: rename cannot replace the destination through it.
	locked, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- Save(&ActivityCache{Activities: []Activity{{Type: ActivityPush}}})
	}()

	time.Sleep(50 * time.Millisecond)
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Save did not return after the lock was released")
	}
}

func TestAddActivityTruncatesToMax(t *testing.T) {
	useTempDir(t)

	base := time.Now()
	for i := range MaxActivities + 25 {
		if err := AddActivity(Activity{
			Type:      ActivityPull,
			Timestamp: base.Add(time.Duration(i) * time.Second),
			RemoteRef: string(rune('a' + i%26)),
		}); err != nil {
			t.Fatalf("AddActivity #%d: %v", i, err)
		}
	}

	got, err := GetAllActivities()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxActivities {
		t.Errorf("got %d activities, want the cap of %d", len(got), MaxActivities)
	}
	// The newest entry must survive truncation.
	if got[0].Type != ActivityPull {
		t.Errorf("newest entry lost after truncation: %+v", got[0])
	}
}

func TestGetRecentActivitiesLimit(t *testing.T) {
	useTempDir(t)

	for i := range 5 {
		if err := AddActivity(Activity{
			Type:      ActivityLabel,
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			RemoteRef: string(rune('a' + i)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct{ limit, want int }{
		{limit: 0, want: 5}, // 0 means all
		{limit: 3, want: 3},
		{limit: 5, want: 5},
		{limit: 99, want: 5},
		{limit: -1, want: 5}, // negative behaves as "all"
	} {
		got, err := GetRecentActivities(tc.limit)
		if err != nil {
			t.Fatalf("limit %d: %v", tc.limit, err)
		}
		if len(got) != tc.want {
			t.Errorf("limit %d: got %d activities, want %d", tc.limit, len(got), tc.want)
		}
	}
}

// GetStats must not hand back a slice aliasing the loaded activities:
// sorting stats.Recent in place would reorder the cache's own data, and
// AddActivity relies on "newest first" ordering to build the next entry.
func TestGetStatsRecentDoesNotAliasLoadedSlice(t *testing.T) {
	useTempDir(t)

	base := time.Now()
	for _, e := range []struct {
		ref string
		off time.Duration
	}{
		{"old", 30 * time.Second},
		{"mid", 10 * time.Second},
		{"new", 20 * time.Second},
	} {
		if err := AddActivity(Activity{
			Type:      ActivityPush,
			Timestamp: base.Add(e.off),
			RemoteRef: e.ref,
		}); err != nil {
			t.Fatal(err)
		}
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	before := append([]Activity(nil), loaded.Activities...)

	stats, err := GetStats(0)
	if err != nil {
		t.Fatal(err)
	}

	// Mutating the result must not reach the loaded cache.
	stats.Recent[0].RemoteRef = "MUTATED"
	for i := range loaded.Activities {
		if loaded.Activities[i].RemoteRef != before[i].RemoteRef {
			t.Errorf("GetStats result aliases the loaded slice at %d: %q -> %q",
				i, before[i].RemoteRef, loaded.Activities[i].RemoteRef)
		}
	}

	// And the persisted file must be unchanged.
	after, err := GetAllActivities()
	if err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if after[i].RemoteRef != before[i].RemoteRef {
			t.Errorf("stored order changed at %d: %q -> %q", i, before[i].RemoteRef, after[i].RemoteRef)
		}
	}
}

func TestGetStatsCountsByType(t *testing.T) {
	useTempDir(t)

	for _, kind := range []ActivityType{ActivityPush, ActivityPush, ActivityPull, ActivityDelete} {
		if err := AddActivity(Activity{Type: kind, RemoteRef: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := GetStats(0)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 4 {
		t.Errorf("Total = %d, want 4", stats.Total)
	}
	want := map[string]int{"push": 2, "pull": 1, "delete": 1}
	for k, v := range want {
		if stats.ByType[k] != v {
			t.Errorf("ByType[%q] = %d, want %d (all: %v)", k, stats.ByType[k], v, stats.ByType)
		}
	}
}

func TestGetStatsSortsRecentNewestFirst(t *testing.T) {
	useTempDir(t)

	base := time.Now()
	for _, e := range []struct {
		ref string
		off time.Duration
	}{
		{"old", 30 * time.Second},
		{"mid", 10 * time.Second},
		{"new", 20 * time.Second},
	} {
		if err := AddActivity(Activity{
			Type:      ActivityPush,
			Timestamp: base.Add(e.off),
			RemoteRef: e.ref,
		}); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := GetStats(0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"old", "new", "mid"} // by timestamp desc
	for i, w := range want {
		if stats.Recent[i].RemoteRef != w {
			t.Errorf("Recent[%d] = %q, want %q", i, stats.Recent[i].RemoteRef, w)
		}
	}
}

func TestClearActivities(t *testing.T) {
	useTempDir(t)

	if err := AddActivity(Activity{Type: ActivityPush, RemoteRef: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearActivities(); err != nil {
		t.Fatalf("ClearActivities: %v", err)
	}

	got, err := GetAllActivities()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d activities after clear, want 0", len(got))
	}
}

func TestLoadCorruptFileFails(t *testing.T) {
	dir := useTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, CacheFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Error("Load on corrupt file = nil, want a parse error")
	}
}

// Save must not leave temp files behind, whether it succeeds or fails.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := useTempDir(t)

	if err := Save(&ActivityCache{Activities: []Activity{{RemoteRef: "x"}}}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 || entries[0].Name() != CacheFileName {
		t.Errorf("unexpected dir contents: %v", entries)
	}
}

func TestSaveIsAtomicOnCorruptTarget(t *testing.T) {
	dir := useTempDir(t)
	path := filepath.Join(dir, CacheFileName)

	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := &ActivityCache{Activities: []Activity{{RemoteRef: "fresh"}}}
	if err := Save(want); err != nil {
		t.Fatalf("Save over corrupt file: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if len(got.Activities) != 1 || got.Activities[0].RemoteRef != "fresh" {
		t.Errorf("got %+v, want the freshly written entry", got.Activities)
	}
}
