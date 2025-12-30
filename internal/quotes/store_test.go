package quotes

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMemoryStore_Random(t *testing.T) {
	t.Run("returns default quote when empty", func(t *testing.T) {
		store := NewMemoryStore(nil)

		quote := store.Random()
		if quote != DefaultQuote {
			t.Errorf("expected default quote, got %q", quote)
		}
	})

	t.Run("returns default quote for empty slice", func(t *testing.T) {
		store := NewMemoryStore([]string{})

		quote := store.Random()
		if quote != DefaultQuote {
			t.Errorf("expected default quote, got %q", quote)
		}
	})

	t.Run("returns single quote", func(t *testing.T) {
		quotes := []string{"only quote"}
		store := NewMemoryStore(quotes)

		quote := store.Random()
		if quote != "only quote" {
			t.Errorf("expected 'only quote', got %q", quote)
		}
	})

	t.Run("returns quotes from store", func(t *testing.T) {
		quotes := []string{"quote1", "quote2", "quote3"}
		store := NewMemoryStore(quotes)

		// Run multiple times to verify randomness
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			quote := store.Random()
			seen[quote] = true
		}

		// Should see at least 2 different quotes with high probability
		if len(seen) < 2 {
			t.Errorf("expected to see multiple quotes, only saw %d", len(seen))
		}

		// All quotes should be from the original set
		for quote := range seen {
			found := false
			for _, q := range quotes {
				if q == quote {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("unexpected quote: %q", quote)
			}
		}
	})
}

func TestMemoryStore_Count(t *testing.T) {
	tests := []struct {
		name     string
		quotes   []string
		expected int
	}{
		{"nil quotes", nil, 0},
		{"empty quotes", []string{}, 0},
		{"one quote", []string{"q1"}, 1},
		{"multiple quotes", []string{"q1", "q2", "q3"}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore(tt.quotes)
			if got := store.Count(); got != tt.expected {
				t.Errorf("Count() = %d, expected %d", got, tt.expected)
			}
		})
	}
}

func TestMemoryStore_Concurrent(t *testing.T) {
	quotes := []string{"q1", "q2", "q3", "q4", "q5"}
	store := NewMemoryStore(quotes)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = store.Random()
				_ = store.Count()
			}
		}()
	}
	wg.Wait()
}

func TestMemoryStore_DefensiveCopy(t *testing.T) {
	quotes := []string{"original"}
	store := NewMemoryStore(quotes)

	// Modify original slice
	quotes[0] = "modified"

	// Store should have the original value
	quote := store.Random()
	if quote != "original" {
		t.Errorf("expected 'original', got %q (defensive copy failed)", quote)
	}
}

func TestLoadFromFile(t *testing.T) {
	t.Run("loads quotes from file", func(t *testing.T) {
		content := "Quote one\nQuote two\nQuote three\n"
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		quotes, err := LoadFromFile(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(quotes) != 3 {
			t.Errorf("expected 3 quotes, got %d", len(quotes))
		}
	})

	t.Run("skips empty lines", func(t *testing.T) {
		content := "Quote one\n\nQuote two\n\n\nQuote three\n"
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		quotes, err := LoadFromFile(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(quotes) != 3 {
			t.Errorf("expected 3 quotes, got %d", len(quotes))
		}
	})

	t.Run("skips comment lines", func(t *testing.T) {
		content := "# This is a comment\nQuote one\n# Another comment\nQuote two\n"
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		quotes, err := LoadFromFile(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(quotes) != 2 {
			t.Errorf("expected 2 quotes, got %d", len(quotes))
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		content := "  Quote with leading space\nQuote with trailing space  \n  Quote with both  \n"
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		quotes, err := LoadFromFile(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{
			"Quote with leading space",
			"Quote with trailing space",
			"Quote with both",
		}

		for i, q := range quotes {
			if q != expected[i] {
				t.Errorf("quote %d: expected %q, got %q", i, expected[i], q)
			}
		}
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := LoadFromFile("/non/existent/file")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("returns empty slice for empty file", func(t *testing.T) {
		tmpFile := createTempFile(t, "")
		defer os.Remove(tmpFile)

		quotes, err := LoadFromFile(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(quotes) != 0 {
			t.Errorf("expected 0 quotes, got %d", len(quotes))
		}
	})
}

func TestDefaultQuotes(t *testing.T) {
	quotes := DefaultQuotes()

	if len(quotes) == 0 {
		t.Error("DefaultQuotes() returned empty slice")
	}

	// Verify quotes are non-empty strings
	for i, q := range quotes {
		if strings.TrimSpace(q) == "" {
			t.Errorf("quote %d is empty or whitespace", i)
		}
	}
}

func TestMustLoadFromFile_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-existent file")
		}
	}()

	MustLoadFromFile("/non/existent/file")
}

// createTempFile creates a temporary file with the given content.
func createTempFile(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "quotes.txt")

	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	return tmpFile
}
