package main

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sync/semaphore"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type Commit struct {
	ID          string `json:"id"`
	AuthorEmail string `json:"author_email"`
}

type DiffStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Total     int `json:"total"`
}

type Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
}

func getChangedLines(projectID int, gitlabURL, privateToken, since string) (map[string]map[string]int, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/repository/commits", gitlabURL, projectID)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", privateToken),
	}
	allChanges := make(map[string]map[string]int)
	page := 1

	for {
		params := fmt.Sprintf("?per_page=100&page=%d", page)
		if since != "" {
			params += "&since=" + since
		}

		resp, err := makeRequest("GET", url+params, headers)
		if err != nil {
			return nil, err
		}

		var commits []Commit
		if err := json.Unmarshal(resp, &commits); err != nil {
			return nil, fmt.Errorf("failed to parse commits: %w", err)
		}
		if len(commits) == 0 {
			break
		}

		for _, commit := range commits {
			commitURL := fmt.Sprintf("%s/api/v4/projects/%d/repository/commits/%s", gitlabURL, projectID, commit.ID)
			diffResp, err := makeRequest("GET", commitURL, headers)
			if err != nil {
				fmt.Printf("Error getting diff for commit %s: %v\n", commit.ID, err)
				continue
			}

			var diffs struct {
				Stats DiffStats `json:"stats"`
			}
			if err := json.Unmarshal(diffResp, &diffs); err != nil {
				fmt.Printf("Error parsing diff stats for commit %s: %v\n", commit.ID, err)
				continue
			}

			if _, ok := allChanges[commit.AuthorEmail]; !ok {
				allChanges[commit.AuthorEmail] = map[string]int{"added": 0, "removed": 0, "total": 0}
			}

			allChanges[commit.AuthorEmail]["added"] += diffs.Stats.Additions
			allChanges[commit.AuthorEmail]["removed"] += diffs.Stats.Deletions
			allChanges[commit.AuthorEmail]["total"] += diffs.Stats.Total
		}
		page++
	}

	return allChanges, nil
}

func getAllProjects(gitlabURL, privateToken, patternToFind string) ([]Project, error) {
	url := fmt.Sprintf("%s/api/v4/projects", gitlabURL)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", privateToken),
	}
	var allProjects []Project

	page := 1

	for {
		params := fmt.Sprintf("?per_page=100&page=%d&simple=true", page)
		resp, err := makeRequest("GET", url+params, headers)
		if err != nil {
			return nil, err
		}

		var projects []Project

		if unmErr := json.Unmarshal(resp, &projects); unmErr != nil {
			return nil, fmt.Errorf("failed to parse projects: %w", unmErr)
		}
		if len(projects) == 0 {
			break
		}
		for _, project := range projects {
			if matched, _ := regexp.MatchString(patternToFind, project.Name+project.PathWithNamespace); matched {
				allProjects = append(allProjects, project)
			}
		}
		page++
	}

	return allProjects, nil
}

func makeRequest(method, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Error closing response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func main() {
	privateToken := os.Getenv("GITLAB_PRIVATE_TOKEN")
	sinceDays, err := strconv.Atoi(os.Getenv("SINCE_DAYS"))
	patternToFind := os.Getenv("PATTERN_TO_FIND")
	concurrencyNumber := os.Getenv("CONCURRENCY_NUMBER")
	gitlabURL := os.Getenv("GITLAB_URL")

	if privateToken == "" {
		fmt.Println("Error: GITLAB_PRIVATE_TOKEN environment variable must be set.")
		os.Exit(1)
	}
	if err != nil || sinceDays <= 0 {
		fmt.Printf("Error parsing SINCE_DAYS: %v. Pick default 360 days\n", err)
		sinceDays = 360
	}
	sinceDate := time.Now().AddDate(0, 0, -sinceDays).Format("2006-01-02")

	if err != nil {
		fmt.Printf("Error fetching projects: %v\n", err)
		os.Exit(1)
	}
	concurrencyNumberConverted, convErr := strconv.Atoi(concurrencyNumber)

	if convErr != nil {
		fmt.Printf("Failed to convert concurrency number to int: %v. Pick default 20\n", convErr)
		concurrencyNumberConverted = 20
	}
	if gitlabURL == "" {
		fmt.Println("Gitlab URL is not set. Using default: https://gitlab.com")
		gitlabURL = "https://gitlab.com"
	}

	allProjects, err := getAllProjects(gitlabURL, privateToken, patternToFind)

	var wg sync.WaitGroup

	allChangesCombined := make(map[string]map[string]int)

	var mu sync.Mutex

	sem := semaphore.NewWeighted(int64(concurrencyNumberConverted))

	for _, project := range allProjects {
		wg.Add(1)
		if acqErr := sem.Acquire(context.Background(), 1); acqErr != nil {
			fmt.Printf("Failed to acquire semaphore: %v\n", acqErr)
			continue
		}
		go func(project Project) {
			defer wg.Done()
			defer sem.Release(1)

			fmt.Printf("Processing project: %s (ID: %d)\n", project.Name, project.ID)
			changes, err := getChangedLines(project.ID, gitlabURL, privateToken, sinceDate)
			if err != nil {
				fmt.Printf("Skipping project %s due to errors: %v\n", project.Name, err)
				return
			}

			mu.Lock()
			for author, counts := range changes {
				if _, ok := allChangesCombined[author]; !ok {
					allChangesCombined[author] = map[string]int{"added": 0, "removed": 0, "total": 0}
				}
				allChangesCombined[author]["added"] += counts["added"]
				allChangesCombined[author]["removed"] += counts["removed"]
				allChangesCombined[author]["total"] += counts["total"]
			}
			mu.Unlock()
		}(project)
	}
	fmt.Println("Waiting to finish all calculations")

	wg.Wait()

	var totalAdded, totalRemoved, total int

	fmt.Println("--- Combined Results ---")

	for author, counts := range allChangesCombined {
		fmt.Printf("Author: %s\nAdded Lines: %d\nRemoved Lines: %d\nTotal Lines: %d\n---\n", author, counts["added"], counts["removed"], counts["total"])
		totalAdded += counts["added"]
		totalRemoved += counts["removed"]
		total += counts["total"]
	}

	fmt.Printf("Total added: %d\nTotal removed: %d\nTotal: %d\n", totalAdded, totalRemoved, total)
}
