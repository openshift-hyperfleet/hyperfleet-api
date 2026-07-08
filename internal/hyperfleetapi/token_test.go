package hyperfleetapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTokenSource_Get(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")

	if err := os.WriteFile(path, []byte("  my-token\n"), 0600); err != nil {
		t.Fatal(err)
	}

	ts := newFileTokenSource(path, 0)
	tok, err := ts.get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "my-token" {
		t.Fatalf("got %q, want %q", tok, "my-token")
	}
}

func TestFileTokenSource_NoCacheReadsFileEveryTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")

	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}

	ts := newFileTokenSource(path, 0) // cacheTTL == 0: no cache

	tok1, err := ts.get()
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(path, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}

	tok2, err := ts.get()
	if err != nil {
		t.Fatal(err)
	}
	if tok1 == tok2 {
		t.Fatalf("expected file to be re-read (no cache), but got same token %q both times", tok1)
	}
}

func TestFileTokenSource_CachesToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")

	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}

	ts := newFileTokenSource(path, time.Minute)
	tok1, err := ts.get()
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite the file — the cache should still return the first value.
	if err = os.WriteFile(path, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}

	tok2, err := ts.get()
	if err != nil {
		t.Fatal(err)
	}
	if tok1 != tok2 {
		t.Fatalf("expected cached token %q, got %q", tok1, tok2)
	}
}

func TestFileTokenSource_RefreshesAfterTTL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")

	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}

	ts := newFileTokenSource(path, time.Minute)
	if _, err := ts.get(); err != nil {
		t.Fatal(err)
	}

	// Expire the cache manually.
	ts.expiresAt = time.Now().Add(-time.Second).UnixNano()

	if err := os.WriteFile(path, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}

	tok, err := ts.get()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "second" {
		t.Fatalf("expected refreshed token %q, got %q", "second", tok)
	}
}

func TestFileTokenSource_MissingFile(t *testing.T) {
	ts := newFileTokenSource("/nonexistent/path/token", 0)
	_, err := ts.get()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFileTokenSource_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")

	if err := os.WriteFile(path, []byte("   \n"), 0600); err != nil {
		t.Fatal(err)
	}

	ts := newFileTokenSource(path, 0)
	_, err := ts.get()
	if err == nil {
		t.Fatal("expected error for empty token file")
	}
}
