# GitLab Lines of Code Counter

This Go program retrieves and aggregates lines of code changed (added, removed, total) across GitLab projects within a specified timeframe. It supports filtering projects by name or path and allows concurrent processing for improved speed.

## Features

* **Retrieves commit data:** Fetches commit details including author email and lines of code changed.
* **Filters projects:**  Allows filtering projects based on a regular expression pattern matched against project name or path.
* **Concurrent processing:** Processes multiple projects concurrently to improve performance.
* **Configurable timeframe:**  Allows specifying the timeframe (in days) for retrieving commit data.
* **Aggregates results:** Combines and displays the total lines of code changed per author across all projects.

## Environment Variables

* **`GITLAB_PRIVATE_TOKEN`:** (Required) Your GitLab private token for API authentication.
* **`SINCE_DAYS`:**  (Optional) Number of days back from today to consider commits. Defaults to 360 days if not set or invalid.
* **`PATTERN_TO_FIND`:** (Optional) Regular expression pattern to filter projects by name or path.
* **`CONCURRENCY_NUMBER`:** (Optional) Number of projects to process concurrently. Defaults to 20 if not set or invalid.
* **`GITLAB_URL`:** (Optional) The URL of your GitLab instance. Defaults to "https://gitlab.com" if not set.
* **`PAGE_SIZE`:** (Optional) Page size to fetch all projects. 100 by default if not set.

## Usage

1. **Set environment variables:**  Ensure the required environment variables are set.
2. **Build and run:**
   ```bash
   go build -o gitlab-lines-counter 
   ./gitlab-lines-counter