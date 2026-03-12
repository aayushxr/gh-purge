package main

import (
	"fmt"
	"os"

	"gh-purge/internal/auth"
	"gh-purge/internal/github"
	"gh-purge/internal/ui"
)

func main() {
	ui.PrintBanner()

	proceed, err := ui.PromptAuthConsent()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if !proceed {
		fmt.Println("Authentication cancelled.")
		os.Exit(0)
	}

	deviceCode, err := auth.RequestDeviceCode()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	ui.PrintDeviceCodeInstructions(deviceCode.UserCode, deviceCode.VerificationURI)

	if err := auth.OpenBrowser(deviceCode.VerificationURI); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to open browser automatically. Please visit the URL above manually.")
	}

	token, err := auth.PollForAccessToken(deviceCode.DeviceCode, deviceCode.Interval, deviceCode.ExpiresIn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	client := github.New(token)

	user, err := client.GetUser()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	ui.PrintAuthSuccess(user.Login)
	ui.PrintFetchingRepos()

	repos, err := client.GetAllRepositories()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	ui.PrintRepoCount(len(repos))

	selected, err := ui.SelectRepositories(repos)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	results, err := ui.ConfirmAndDelete(selected, repos, client.DeleteRepository)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	ui.DisplayDeletionResults(user, results)
}
