package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"gh-purge/internal/github"
)

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

func PrintBanner() {
	fmt.Println(titleStyle.Render("🗑️  GitHub Purge"))
	fmt.Println("Authenticate with GitHub and select repositories to delete")
}

func PromptAuthConsent() (bool, error) {
	var proceed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("🔐 GitHub Authentication").
				Description("This will open GitHub in your browser to authenticate.\nWould you like to proceed?").
				Value(&proceed),
		),
	)
	if err := form.Run(); err != nil {
		return false, fmt.Errorf("form error: %w", err)
	}
	return proceed, nil
}

func PrintDeviceCodeInstructions(userCode, verificationURI string) {
	fmt.Println(infoStyle.Render(fmt.Sprintf("\n📋 Your device code: %s", userCode)))
	fmt.Println(infoStyle.Render(fmt.Sprintf("🌐 Opening: %s", verificationURI)))
	fmt.Println(infoStyle.Render("\n⏳ Waiting for authentication..."))
}

func PrintAuthSuccess(login string) {
	fmt.Println(successStyle.Render(fmt.Sprintf("✅ Successfully authenticated as %s!", login)))
}

func PrintFetchingRepos() {
	fmt.Println(infoStyle.Render("\n📚 Fetching your repositories..."))
}

func PrintRepoCount(count int) {
	fmt.Println(successStyle.Render(fmt.Sprintf("✅ Found %d repositories", count)))
}

func SelectRepositories(repos []github.Repo) ([]string, error) {
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found")
	}

	options := make([]huh.Option[string], len(repos))
	for i, repo := range repos {
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

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("🗑️  Select Repositories to DELETE").
				Description(fmt.Sprintf("⚠️  WARNING: Selected repositories will be PERMANENTLY DELETED!\nChoose from your %d repositories (use space to select, enter to confirm):", len(repos))).
				Options(options...).
				Value(&selected).
				Filterable(true),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}

	return selected, nil
}

func ConfirmAndDelete(selectedFullNames []string, allRepos []github.Repo, deleteFunc func(string) error) ([]github.DeletionResult, error) {
	if len(selectedFullNames) == 0 {
		fmt.Println(infoStyle.Render("No repositories selected for deletion."))
		return nil, nil
	}

	fmt.Println(errorStyle.Render("\n🚨 DANGER ZONE 🚨"))
	fmt.Println(errorStyle.Render("The following repositories will be PERMANENTLY DELETED:"))

	for i, repoName := range selectedFullNames {
		for _, repo := range allRepos {
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

	var firstConfirm bool
	form1 := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("⚠️  Are you absolutely sure?").
				Description(fmt.Sprintf("This will permanently delete %d repositories. This action cannot be undone!", len(selectedFullNames))).
				Value(&firstConfirm),
		),
	)
	if err := form1.Run(); err != nil {
		return nil, err
	}
	if !firstConfirm {
		fmt.Println(infoStyle.Render("Deletion cancelled."))
		return nil, nil
	}

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
		return nil, err
	}

	fmt.Println(errorStyle.Render("\n🗑️  Starting deletion process..."))

	var results []github.DeletionResult
	for i, repoName := range selectedFullNames {
		fmt.Printf("Deleting %d/%d: %s... ", i+1, len(selectedFullNames), repoName)

		err := deleteFunc(repoName)
		result := github.DeletionResult{
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

		results = append(results, result)
		time.Sleep(500 * time.Millisecond)
	}

	return results, nil
}

func DisplayDeletionResults(user github.User, results []github.DeletionResult) {
	fmt.Println(titleStyle.Render("\n🗑️  Deletion Complete!"))

	if len(results) == 0 {
		fmt.Println(infoStyle.Render("No repositories were deleted."))
		return
	}

	successCount := 0
	failureCount := 0

	fmt.Println(infoStyle.Render(fmt.Sprintf("User: %s", user.Login)))
	fmt.Println(infoStyle.Render("Deletion Results:"))

	for _, result := range results {
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
