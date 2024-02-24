package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
)

type Repository struct {
	Name string `json:"name"`
}

type Repositories struct {
	Items []Repository `json:"items"`
}

func deleteRepositories(repos []string, accessToken string, userName string) (bool, error) {
	for _, repo := range repos {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s", userName, repo)
		req, err := http.NewRequest("DELETE", url, nil)
		if err != nil {
			return false, err
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			return false, fmt.Errorf("failed to delete repository: %s", resp.Status)
		}
	}

	return true, nil

}

func getRepositoryNames(user User) ([]string, error) {
	// TODO: get private repositories too
	url := fmt.Sprintf("https://api.github.com/users/%s/repos", user.Name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", user.AccessToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get repositories: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var repositories []Repository
	err = json.Unmarshal(body, &repositories)
	if err != nil {
		return nil, err
	}

	var repositoryNames []string
	for _, repo := range repositories {
		repositoryNames = append(repositoryNames, repo.Name)
	}

	return repositoryNames, nil
}

type User struct {
	Name        string
	AccessToken string
}

func main() {
	var user = User{}
	var selectedrepos = []string{}
	// Should we run in accessible mode?
	accessible, _ := strconv.ParseBool(os.Getenv("ACCESSIBLE"))

	form := huh.NewForm(
		huh.NewGroup(huh.NewNote().
			Title("Github Purge").
			Description("Purge your unwanted github repos with ease")),

		huh.NewGroup(
			huh.NewInput().Title("Github Username").Value(&user.Name).Placeholder("Enter your github username"),
			huh.NewInput().Title("Github Access token").Description("You can get it by going to https://github.com/settings/tokens").Value(&user.AccessToken).Placeholder("ghp_xxxx"),
		),
	).WithAccessible(accessible)

	err := form.Run()

	if err != nil {
		fmt.Println("Uh oh:", err)
		os.Exit(1)
	}

	actionFunc := func() {
		_, err := getRepositoryNames(user)
		if err != nil {
			fmt.Println("Uh oh:", err)
			os.Exit(1)
		}
	}

	_ = spinner.New().Title("Getting Your Repos...").Accessible(accessible).Action(actionFunc).Run()

	repos, err := getRepositoryNames(user)
	if err != nil {
		fmt.Println("Uh oh:", err)
		os.Exit(1)
	}

	options := []huh.Option[string]{}
	for _, repo := range repos {
		options = append(options, huh.NewOption(repo, repo))
	}

	form2 := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().Title("Select Repos").Options(options...).Value(&selectedrepos),
		),
	).WithAccessible(accessible)

	err = form2.Run()

	if err != nil {
		fmt.Println("Uh oh:", err)
		os.Exit(1)
	}

	purgeFunc := func() {
		_, err := deleteRepositories(selectedrepos, user.AccessToken, user.Name)
		if err != nil {
			fmt.Println("Uh oh:", err)
			os.Exit(1)
		}
	}

	_ = spinner.New().Title("Purging your repos...").Accessible(accessible).Action(purgeFunc).Run()

	// Print order summary.
	{
		var sb strings.Builder

		fmt.Fprintf(&sb,

			"Repositories purged",
		)

		fmt.Println(
			lipgloss.NewStyle().
				Width(40).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(1, 2).
				Render(sb.String()),
		)
	}
}
