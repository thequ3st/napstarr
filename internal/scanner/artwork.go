package scanner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dhowden/tag"
)

// ExtractArtwork reads embedded artwork from an audio file and writes it to disk.
// Skips if artwork already exists for the given album.
func ExtractArtwork(f *os.File, albumID string, artworkDir string) error {
	outPath := filepath.Join(artworkDir, albumID+".jpg")

	// Skip if already extracted.
	if _, err := os.Stat(outPath); err == nil {
		return nil
	}

	// Seek to beginning in case file was already read.
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek: %w", err)
	}

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil // no tags — nothing to extract
	}

	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil // no embedded artwork
	}

	if err := os.MkdirAll(artworkDir, 0755); err != nil {
		return fmt.Errorf("create artwork dir: %w", err)
	}

	if err := os.WriteFile(outPath, pic.Data, 0644); err != nil {
		return fmt.Errorf("write artwork: %w", err)
	}

	return nil
}
