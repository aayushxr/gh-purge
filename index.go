package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// GitHub OAuth Device Flow structs
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

type GitHubRepo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"html_url"`
}

type GitHubUser struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

const (
	clientID = "Ov23liP6o0infpaUR8Eg"
)

var (
	accessToken       string
	selectedRepos     []string
	availableRepos    []GitHubRepo
	authenticatedUser GitHubUser
	deletionResults   []DeletionResult
)

type DeletionResult struct {
	RepoName string
	Success  bool
	Error    string
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true).
			Padding(1, 2)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))
)

func main() {
	fmt.Println(titleStyle.Render("🗑️  GitHub Purge"))
	fmt.Println("Authenticate with GitHub and select repositories to delete")

	// Step 1: GitHub Authentication
	if err := authenticateWithGitHub(); err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("❌ Authentication failed: %v", err)))
		os.Exit(1)
	}

	// Step 2: Fetch repositories
	if err := fetchRepositories(); err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("❌ Failed to fetch repositories: %v", err)))
		os.Exit(1)
	}

	// Step 3: Repository selection
	if err := selectRepositoriesToDelete(); err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("❌ Repository selection failed: %v", err)))
		os.Exit(1)
	}

	// Step 4: Final confirmation and deletion
	if err := confirmAndDeleteRepositories(); err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("❌ Deletion failed: %v", err)))
		os.Exit(1)
	}

	// Display results
	displayDeletionResults()
}

func authenticateWithGitHub() error {
	var proceed bool

	// Initial form to start authentication
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("🔐 GitHub Authentication").
				Description("This will open GitHub in your browser to authenticate.\nWould you like to proceed?").
				Value(&proceed),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("form error: %w", err)
	}

	if !proceed {
		return fmt.Errorf("authentication cancelled by user")
	}

	// Start OAuth device flow
	deviceCode, err := requestDeviceCode()
	if err != nil {
		return fmt.Errorf("failed to get device code: %w", err)
	}

	// Display user code and open browser
	fmt.Println(infoStyle.Render(fmt.Sprintf("\n📋 Your device code: %s", deviceCode.UserCode)))
	fmt.Println(infoStyle.Render(fmt.Sprintf("🌐 Opening: %s", deviceCode.VerificationURI)))

	if err := openBrowser(deviceCode.VerificationURI); err != nil {
		fmt.Println(errorStyle.Render("Failed to open browser automatically"))
		fmt.Println(infoStyle.Render(fmt.Sprintf("Please manually visit: %s", deviceCode.VerificationURI)))
	}

	fmt.Println(infoStyle.Render("\n⏳ Waiting for authentication..."))

	// Poll for access token
	token, err := pollForAccessToken(deviceCode.DeviceCode, deviceCode.Interval, deviceCode.ExpiresIn)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	accessToken = token

	// Get user info
	user, err := getUserInfo(accessToken)
	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}

	authenticatedUser = user
	fmt.Println(successStyle.Render(fmt.Sprintf("✅ Successfully authenticated as %s!", user.Login)))

	return nil
}

func requestDeviceCode() (*DeviceCodeResponse, error) {
	url := "https://github.com/login/device/code"
	data := fmt.Sprintf("client_id=%s&scope=repo,delete_repo", clientID)

	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub device code request failed: %s", string(body))
	}

	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(strings.NewReader(string(body))).Decode(&deviceResp); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	// Validate response
	if deviceResp.DeviceCode == "" || deviceResp.UserCode == "" || deviceResp.VerificationURI == "" {
		return nil, fmt.Errorf("invalid device code response from GitHub")
	}

	// Set default values if not provided
	if deviceResp.Interval == 0 {
		deviceResp.Interval = 5 // Default to 5 seconds
	}
	if deviceResp.ExpiresIn == 0 {
		deviceResp.ExpiresIn = 900 // Default to 15 minutes
	}

	return &deviceResp, nil
}

func pollForAccessToken(deviceCode string, interval, expiresIn int) (string, error) {
	url := "https://github.com/login/oauth/access_token"
	data := fmt.Sprintf("client_id=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code", clientID, deviceCode)

	// Ensure minimum interval to prevent panic
	if interval < 1 {
		interval = 5
	}

	timeout := time.Now().Add(time.Duration(expiresIn) * time.Second)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// fmt.Println(infoStyle.Render(fmt.Sprintf("Polling every %d seconds...", interval)))

	for {
		select {
		case <-ticker.C:
			if time.Now().After(timeout) {
				return "", fmt.Errorf("authentication timeout")
			}

			req, err := http.NewRequest("POST", url, strings.NewReader(data))
			if err != nil {
				continue
			}

			req.Header.Set("Accept", "application/json")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				// fmt.Println(infoStyle.Render("Retrying..."))
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			var tokenResp AccessTokenResponse
			if err := json.Unmarshal(body, &tokenResp); err != nil {
				continue
			}

			if tokenResp.Error == "authorization_pending" {
				fmt.Print(".")
				continue
			} else if tokenResp.Error == "slow_down" {
				ticker.Reset(time.Duration(interval+5) * time.Second)
				// fmt.Println(infoStyle.Render("\nSlowing down polling rate..."))
				continue
			} else if tokenResp.Error == "expired_token" {
				return "", fmt.Errorf("device code expired, please try again")
			} else if tokenResp.Error == "access_denied" {
				return "", fmt.Errorf("access denied by user")
			} else if tokenResp.Error != "" {
				return "", fmt.Errorf("oauth error: %s", tokenResp.Error)
			} else if tokenResp.AccessToken != "" {
				fmt.Println() // New line after dots
				return tokenResp.AccessToken, nil
			}
		}
	}
}

func getUserInfo(token string) (GitHubUser, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return GitHubUser{}, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return GitHubUser{}, err
	}
	defer resp.Body.Close()

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return GitHubUser{}, err
	}

	return user, nil
}

func fetchRepositories() error {
	fmt.Println(infoStyle.Render("\n📚 Fetching your repositories..."))

	repos, err := getAllRepositories(accessToken)
	if err != nil {
		return err
	}

	availableRepos = repos
	fmt.Println(successStyle.Render(fmt.Sprintf("✅ Found %d repositories", len(repos))))

	return nil
}

func getAllRepositories(token string) ([]GitHubRepo, error) {
	var allRepos []GitHubRepo
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/user/repos?page=%d&per_page=%d&sort=updated", page, perPage)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("GitHub API error: %s", string(body))
		}

		var repos []GitHubRepo
		if err := json.Unmarshal(body, &repos); err != nil {
			return nil, err
		}

		allRepos = append(allRepos, repos...)

		// If we got fewer repos than requested, we're on the last page
		if len(repos) < perPage {
			break
		}
		page++
	}

	return allRepos, nil
}

func selectRepositoriesToDelete() error {
	if len(availableRepos) == 0 {
		return fmt.Errorf("no repositories found")
	}

	// Create options for huh multiselect
	options := make([]huh.Option[string], len(availableRepos))
	for i, repo := range availableRepos {
		description := repo.Description
		if description == "" {
			description = "No description"
		}

		visibility := "Public"
		if repo.Private {
			visibility = "Private"
		}

		label := fmt.Sprintf("%s (%s) - %s", repo.Name, visibility, description)
		if len(label) > 80 {
			label = label[:77] + "..."
		}

		options[i] = huh.NewOption(label, repo.FullName)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("🗑️  Select Repositories to DELETE").
				Description(fmt.Sprintf("⚠️  WARNING: Selected repositories will be PERMANENTLY DELETED!\nChoose from your %d repositories (use space to select, enter to confirm):", len(availableRepos))).
				Options(options...).
				Value(&selectedRepos).
				Filterable(true),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	return nil
}

func confirmAndDeleteRepositories() error {
	if len(selectedRepos) == 0 {
		fmt.Println(infoStyle.Render("No repositories selected for deletion."))
		return nil
	}

	// Show what will be deleted
	fmt.Println(errorStyle.Render("\n🚨 DANGER ZONE 🚨"))
	fmt.Println(errorStyle.Render("The following repositories will be PERMANENTLY DELETED:"))

	for i, repoName := range selectedRepos {
		for _, repo := range availableRepos {
			if repo.FullName == repoName {
				visibility := "🔓 Public"
				if repo.Private {
					visibility = "🔒 Private"
				}
				fmt.Printf("  %d. %s %s\n", i+1, visibility, repo.Name)
				break
			}
		}
	}

	// First confirmation
	var firstConfirm bool
	form1 := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("⚠️  Are you absolutely sure?").
				Description(fmt.Sprintf("This will permanently delete %d repositories. This action cannot be undone!", len(selectedRepos))).
				Value(&firstConfirm),
		),
	)

	if err := form1.Run(); err != nil {
		return err
	}

	if !firstConfirm {
		fmt.Println(infoStyle.Render("Deletion cancelled."))
		return nil
	}

	// Second confirmation with typing
	var confirmText string
	form2 := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Type 'delete' to confirm").
				Description("Type delete in all caps to proceed with the deletion:").
				Value(&confirmText).
				Validate(func(s string) error {
					if s != "delete" {
						return fmt.Errorf("you must type 'delete' exactly")
					}
					return nil
				}),
		),
	)

	if err := form2.Run(); err != nil {
		return err
	}

	// Proceed with deletion
	fmt.Println(errorStyle.Render("\n🗑️  Starting deletion process..."))

	for i, repoName := range selectedRepos {
		fmt.Printf("Deleting %d/%d: %s... ", i+1, len(selectedRepos), repoName)

		err := deleteRepository(repoName)
		result := DeletionResult{
			RepoName: repoName,
			Success:  err == nil,
		}

		if err != nil {
			result.Error = err.Error()
			fmt.Println(errorStyle.Render("FAILED"))
			fmt.Println(errorStyle.Render(fmt.Sprintf("  Error: %v", err)))
		} else {
			fmt.Println(successStyle.Render("SUCCESS"))
		}

		deletionResults = append(deletionResults, result)

		// Small delay between deletions to be respectful to GitHub's API
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

func deleteRepository(fullName string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s", fullName)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return nil // Success
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
}

func displayDeletionResults() {
	fmt.Println(titleStyle.Render("\n🗑️  Deletion Complete!"))

	if len(deletionResults) == 0 {
		fmt.Println(infoStyle.Render("No repositories were deleted."))
		return
	}

	successCount := 0
	failureCount := 0

	fmt.Println(infoStyle.Render(fmt.Sprintf("User: %s", authenticatedUser.Login)))
	fmt.Println(infoStyle.Render("Deletion Results:"))

	for _, result := range deletionResults {
		if result.Success {
			fmt.Printf("  ✅ %s - Successfully deleted\n", result.RepoName)
			successCount++
		} else {
			fmt.Printf("  ❌ %s - Failed: %s\n", result.RepoName, result.Error)
			failureCount++
		}
	}

	fmt.Printf("\n")
	fmt.Println(successStyle.Render(fmt.Sprintf("✅ Successfully deleted: %d repositories", successCount)))

	if failureCount > 0 {
		fmt.Println(errorStyle.Render(fmt.Sprintf("❌ Failed to delete: %d repositories", failureCount)))
	}
}

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
