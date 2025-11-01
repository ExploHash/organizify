package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const apiBaseURL = "https://api.spotify.com/v1"

// Client wraps HTTP client with Spotify API helper methods
type Client struct {
	httpClient  *http.Client
	accessToken string
	ctx         context.Context
}

// NewClient creates a new Spotify client wrapper
func NewClient(ctx context.Context) (*Client, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	return &Client{
		httpClient:  &http.Client{},
		accessToken: accessToken,
		ctx:         ctx,
	}, nil
}

// makeRequest makes an authenticated request to the Spotify API
func (c *Client) makeRequest(method, endpoint string, params url.Values) ([]byte, error) {
	urlStr := apiBaseURL + endpoint
	if params != nil {
		urlStr += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(c.ctx, method, urlStr, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// Playlist represents a Spotify playlist
type Playlist struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Tracks struct {
		Total int `json:"total"`
	} `json:"tracks"`
	Owner struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Public       bool   `json:"public"`
	Collaborative bool  `json:"collaborative"`
}

// Track represents a Spotify track
type Track struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name string `json:"name"`
	} `json:"album"`
	DurationMs int `json:"duration_ms"`
}

// SavedTrack represents a saved track with metadata
type SavedTrack struct {
	AddedAt string `json:"added_at"`
	Track   Track  `json:"track"`
}

// User represents a Spotify user
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

// GetAllPlaylists fetches all user playlists with automatic pagination
func (c *Client) GetAllPlaylists() ([]Playlist, error) {
	var allPlaylists []Playlist
	limit := 50
	offset := 0

	for {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", limit))
		params.Set("offset", fmt.Sprintf("%d", offset))

		body, err := c.makeRequest("GET", "/me/playlists", params)
		if err != nil {
			return nil, fmt.Errorf("failed to get playlists: %w", err)
		}

		var response struct {
			Items []Playlist `json:"items"`
			Total int        `json:"total"`
			Next  *string    `json:"next"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse playlists: %w", err)
		}

		allPlaylists = append(allPlaylists, response.Items...)

		if response.Next == nil || len(response.Items) < limit {
			break
		}
		offset += limit
	}

	return allPlaylists, nil
}

// GetLikedSongs fetches all user's liked songs (saved tracks) with automatic pagination
func (c *Client) GetLikedSongs() ([]SavedTrack, error) {
	var allTracks []SavedTrack
	limit := 50
	offset := 0

	for {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", limit))
		params.Set("offset", fmt.Sprintf("%d", offset))

		body, err := c.makeRequest("GET", "/me/tracks", params)
		if err != nil {
			return nil, fmt.Errorf("failed to get liked songs: %w", err)
		}

		var response struct {
			Items []SavedTrack `json:"items"`
			Total int          `json:"total"`
			Next  *string      `json:"next"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse liked songs: %w", err)
		}

		allTracks = append(allTracks, response.Items...)

		if response.Next == nil || len(response.Items) < limit {
			break
		}
		offset += limit
	}

	return allTracks, nil
}

// GetPlaylistTracks fetches all tracks from a specific playlist with automatic pagination
func (c *Client) GetPlaylistTracks(playlistID string) ([]Track, error) {
	var allTracks []Track
	limit := 100
	offset := 0

	for {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", limit))
		params.Set("offset", fmt.Sprintf("%d", offset))

		endpoint := fmt.Sprintf("/playlists/%s/tracks", playlistID)
		body, err := c.makeRequest("GET", endpoint, params)
		if err != nil {
			return nil, fmt.Errorf("failed to get playlist tracks: %w", err)
		}

		var response struct {
			Items []struct {
				Track Track `json:"track"`
			} `json:"items"`
			Total int     `json:"total"`
			Next  *string `json:"next"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse playlist tracks: %w", err)
		}

		for _, item := range response.Items {
			allTracks = append(allTracks, item.Track)
		}

		if response.Next == nil || len(response.Items) < limit {
			break
		}
		offset += limit
	}

	return allTracks, nil
}

// GetCurrentUser fetches the current user's profile
func (c *Client) GetCurrentUser() (*User, error) {
	body, err := c.makeRequest("GET", "/me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
}

// GetPlaylistByName searches for a playlist by name in the user's playlists
func (c *Client) GetPlaylistByName(name string) (*Playlist, error) {
	playlists, err := c.GetAllPlaylists()
	if err != nil {
		return nil, err
	}

	for _, playlist := range playlists {
		if playlist.Name == name {
			return &playlist, nil
		}
	}

	return nil, fmt.Errorf("playlist '%s' not found", name)
}

// GetLikedSongsCount returns the total number of liked songs
func (c *Client) GetLikedSongsCount() (int, error) {
	params := url.Values{}
	params.Set("limit", "1")

	body, err := c.makeRequest("GET", "/me/tracks", params)
	if err != nil {
		return 0, fmt.Errorf("failed to get liked songs count: %w", err)
	}

	var response struct {
		Total int `json:"total"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Total, nil
}

// GetPlaylistsCount returns the total number of playlists
func (c *Client) GetPlaylistsCount() (int, error) {
	params := url.Values{}
	params.Set("limit", "1")

	body, err := c.makeRequest("GET", "/me/playlists", params)
	if err != nil {
		return 0, fmt.Errorf("failed to get playlists count: %w", err)
	}

	var response struct {
		Total int `json:"total"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Total, nil
}