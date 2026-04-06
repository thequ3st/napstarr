package database

import (
	"crypto/rand"
	"fmt"
	"time"
)

type User struct {
	ID           string `json:"id"`
	InstanceID   string `json:"instanceId"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	PasswordHash string `json:"-"`
	IsAdmin      bool   `json:"isAdmin"`
	CreatedAt    string `json:"createdAt"`
}

type Artist struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SortName      string `json:"sortName"`
	MusicBrainzID string `json:"musicbrainzId,omitempty"`
	AlbumCount    int    `json:"albumCount,omitempty"`
	TrackCount    int    `json:"trackCount,omitempty"`
}

type Album struct {
	ID            string  `json:"id"`
	ArtistID      string  `json:"artistId"`
	ArtistName    string  `json:"artistName,omitempty"`
	Title         string  `json:"title"`
	Year          *int    `json:"year,omitempty"`
	Genre         string  `json:"genre,omitempty"`
	DiscTotal     int     `json:"discTotal"`
	MusicBrainzID string  `json:"musicbrainzId,omitempty"`
	HasArtwork    bool    `json:"hasArtwork"`
	TrackCount    int     `json:"trackCount,omitempty"`
	Tracks        []Track `json:"tracks,omitempty"`
}

type Track struct {
	ID           string `json:"id"`
	AlbumID      string `json:"albumId"`
	ArtistID     string `json:"artistId"`
	ArtistName   string `json:"artistName,omitempty"`
	AlbumTitle   string `json:"albumTitle,omitempty"`
	Title        string `json:"title"`
	TrackNumber  *int   `json:"trackNumber,omitempty"`
	DiscNumber   int    `json:"discNumber"`
	DurationMs   int    `json:"durationMs"`
	FilePath     string `json:"-"`
	FileSize     int64  `json:"fileSize"`
	Format       string `json:"format"`
	Bitrate      *int   `json:"bitrate,omitempty"`
	SampleRate   *int   `json:"sampleRate,omitempty"`
}

type ListenHistory struct {
	ID         string `json:"id"`
	UserID     string `json:"userId"`
	TrackID    string `json:"trackId"`
	Track      *Track `json:"track,omitempty"`
	ListenedAt string `json:"listenedAt"`
	DurationMs int    `json:"durationMs"`
}

type LibraryStats struct {
	ArtistCount int `json:"artistCount"`
	AlbumCount  int `json:"albumCount"`
	TrackCount  int `json:"trackCount"`
	TotalSizeMB int `json:"totalSizeMb"`
}

// NewID generates a UUIDv7 (time-sortable, globally unique).
func NewID() string {
	now := time.Now()
	ms := now.UnixMilli()

	var id [16]byte
	// Encode 48-bit timestamp manually (BigEndian.PutUint48 doesn't exist)
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	rand.Read(id[6:])

	// Set version 7
	id[6] = (id[6] & 0x0F) | 0x70
	// Set variant 2
	id[8] = (id[8] & 0x3F) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}
