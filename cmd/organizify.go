package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ExploHash/organizify/pkg/spotify"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Organizify - Spotify Authentication Test ===\n")

	// Test authentication and create client
	fmt.Println("Authenticating with Spotify...")
	spotifyClient, err := spotify.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to authenticate: %v", err)
	}

	// Get current user
	user, err := spotifyClient.GetCurrentUser()
	if err != nil {
		log.Fatalf("Failed to get user: %v", err)
	}
	fmt.Printf("\n✓ Logged in as: %s\n", user.DisplayName)

	// Get playlists count
	playlistCount, err := spotifyClient.GetPlaylistsCount()
	if err != nil {
		log.Fatalf("Failed to get playlists count: %v", err)
	}
	fmt.Printf("✓ Total playlists: %d\n", playlistCount)

	// Get liked songs count
	likedCount, err := spotifyClient.GetLikedSongsCount()
	if err != nil {
		log.Fatalf("Failed to get liked songs count: %v", err)
	}
	fmt.Printf("✓ Total liked songs: %d\n", likedCount)

	// Fetch all playlists
	fmt.Println("\nFetching all playlists...")
	playlists, err := spotifyClient.GetAllPlaylists()
	if err != nil {
		log.Fatalf("Failed to get playlists: %v", err)
	}

	fmt.Printf("\nYour Playlists (%d total):\n", len(playlists))
	for i, playlist := range playlists {
		if i >= 10 {
			fmt.Printf("... and %d more\n", len(playlists)-10)
			break
		}
		fmt.Printf("  %d. %s - %d tracks\n", i+1, playlist.Name, playlist.Tracks.Total)
	}

	// Fetch first 10 liked songs
	fmt.Println("\nFetching liked songs (first 10)...")
	likedSongs, err := spotifyClient.GetLikedSongs()
	if err != nil {
		log.Fatalf("Failed to get liked songs: %v", err)
	}

	fmt.Printf("\nYour Liked Songs (showing 10 of %d):\n", len(likedSongs))
	for i, item := range likedSongs {
		if i >= 10 {
			break
		}
		track := item.Track
		artists := ""
		for j, artist := range track.Artists {
			if j > 0 {
				artists += ", "
			}
			artists += artist.Name
		}
		fmt.Printf("  %d. %s - %s\n", i+1, track.Name, artists)
	}

	fmt.Println("\n✓ Test completed successfully!")
}
