package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	clientID    = "e2d7b802ac6a4132a265fab71f0645d0"
	redirectURI = "http://127.0.0.1:1069"
	authURL     = "https://accounts.spotify.com/authorize"
	tokenURL    = "https://accounts.spotify.com/api/token"
)

var (
	ch            = make(chan *oauth2.Token)
	state         string
	codeVerifier  string
	codeChallenge string
	cachedToken   *oauth2.Token
)

// Scopes needed for playlists and liked songs
var scopes = []string{
	"playlist-read-private",
	"playlist-read-collaborative",
	"user-library-read",
}

// generateRandomString generates a cryptographically secure random string
func generateRandomString(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// generateCodeVerifier generates a code verifier for PKCE
func generateCodeVerifier() string {
	return generateRandomString(32)
}

// generateCodeChallenge generates a code challenge from the verifier
func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// initPKCE initializes PKCE parameters
func initPKCE() {
	state = generateRandomString(16)
	codeVerifier = generateCodeVerifier()
	codeChallenge = generateCodeChallenge(codeVerifier)
}

// buildAuthURL builds the authorization URL with PKCE parameters
func buildAuthURL() string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	params.Set("scope", strings.Join(scopes, " "))
	params.Set("code_challenge_method", "S256")
	params.Set("code_challenge", codeChallenge)

	return authURL + "?" + params.Encode()
}

// openBrowser opens the URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// exchangeCodeForToken exchanges the authorization code for an access token
func exchangeCodeForToken(code string) (*oauth2.Token, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", codeVerifier)

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	return token, nil
}

// refreshToken refreshes an expired token
func refreshToken(oldToken *oauth2.Token) (*oauth2.Token, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", oldToken.RefreshToken)

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// If no new refresh token is provided, keep the old one
	newRefreshToken := tokenResp.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = oldToken.RefreshToken
	}

	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: newRefreshToken,
		Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	return token, nil
}

// isTokenValid checks if the token is still valid
func isTokenValid(token *oauth2.Token) bool {
	if token == nil {
		return false
	}
	// Check if token expires in less than 5 minutes
	return token.Expiry.After(time.Now().Add(5 * time.Minute))
}

// completeAuthHandler handles the OAuth callback
func completeAuthHandler(w http.ResponseWriter, r *http.Request) {
	// Check for errors in the callback
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		http.Error(w, fmt.Sprintf("Authentication error: %s - %s", errMsg, errDesc), http.StatusForbidden)
		return
	}

	// Verify state
	if st := r.URL.Query().Get("state"); st != state {
		http.Error(w, "Invalid state parameter", http.StatusForbidden)
		return
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No authorization code", http.StatusBadRequest)
		return
	}

	// Exchange code for token
	token, err := exchangeCodeForToken(code)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get token: %v", err), http.StatusInternalServerError)
		return
	}

	// Send success message to browser
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Organizify - Authentication Successful</title>
			<style>
				body {
					font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
					display: flex;
					justify-content: center;
					align-items: center;
					height: 100vh;
					margin: 0;
					background: linear-gradient(135deg, #1DB954 0%%, #191414 100%%);
				}
				.container {
					background: white;
					padding: 3rem;
					border-radius: 12px;
					box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
					text-align: center;
				}
				h1 { color: #1DB954; margin-bottom: 1rem; }
				p { color: #666; margin: 0.5rem 0; }
			</style>
		</head>
		<body>
			<div class="container">
				<h1>✓ Authentication Successful!</h1>
				<p>You can now close this window and return to your terminal.</p>
			</div>
		</body>
		</html>
	`)

	ch <- token
}

// Login authenticates the user with Spotify and returns access and refresh tokens
// It uses in-memory token caching and automatic refresh
func Login() (accessToken string, refToken string, err error) {
	// Check if we have a cached token in memory
	if cachedToken != nil && isTokenValid(cachedToken) {
		return cachedToken.AccessToken, cachedToken.RefreshToken, nil
	}

	// If token exists but is expired, try to refresh
	if cachedToken != nil && !isTokenValid(cachedToken) {
		newToken, err := refreshToken(cachedToken)
		if err == nil {
			cachedToken = newToken
			return newToken.AccessToken, newToken.RefreshToken, nil
		}
	}

	// Initialize PKCE for authentication
	initPKCE()

	// Start local server to handle callback
	http.HandleFunc("/", completeAuthHandler)
	server := &http.Server{Addr: ":1069"}

	go func() {
		server.ListenAndServe()
	}()

	// Generate auth URL
	url := buildAuthURL()
	fmt.Println("\n=== Spotify Authentication Required ===")
	fmt.Println("Opening browser for authentication...")
	fmt.Println()
	
	// Try to open browser automatically
	if err := openBrowser(url); err != nil {
		fmt.Println("Could not open browser automatically. Please visit this URL:")
		fmt.Println(url)
	} else {
		fmt.Println("If browser doesn't open, visit this URL:")
		fmt.Println(url)
	}
	
	fmt.Println()
	fmt.Println("Waiting for authentication...")

	// Wait for auth to complete with timeout
	var token *oauth2.Token
	select {
	case token = <-ch:
		// Authentication successful
	case <-time.After(5 * time.Minute):
		server.Shutdown(context.Background())
		return "", "", fmt.Errorf("authentication timeout after 5 minutes")
	}

	// Shutdown the server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)

	// Cache token in memory
	cachedToken = token

	fmt.Println("✓ Authentication successful!")
	return token.AccessToken, token.RefreshToken, nil
}

// GetAccessToken returns a valid access token, handling refresh automatically
func GetAccessToken() (string, error) {
	accessToken, _, err := Login()
	return accessToken, err
}