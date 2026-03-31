Write a Go program that connects to Azure DevOps and scans all Git repositories
across all projects in an organization. The program should use the Azure DevOps
REST API with a Personal Access Token (PAT) for authentication.

## Requirements

### Configuration

- Read the following from environment variables:
  - AZURE_DEVOPS_ORG (organization name)
  - AZURE_DEVOPS_PAT (personal access token)
  - INACTIVE_DAYS_THRESHOLD (default to 90 if not set)

### Core Features

#### 1. List All Repositories

- Iterate through ALL projects in the organization
- For each project, list ALL git repositories
- Handle pagination (Azure DevOps API returns max 100 items per page)
- Skip disabled repositories (flag them but still report them)

#### 2. Pipeline Template Detection

For each repository, on the master/main/default branch:

- Search for files matching the glob pattern: `pipeline/*.pipeline.yaml`
  or `pipeline/*.pipeline.yml`
- Use the Azure DevOps Git Items API to list files in the `pipeline/` folder:
  GET <https://dev.azure.com/{org}/{project}/_apis/git/repositories/{repoId}/items>
  ?scopePath=/pipeline
  &recursionLevel=OneLevel
  &api-version=7.0
- For each matched pipeline file, fetch its raw content
- Parse the YAML content to extract the value at `extends.pipeline`
- If multiple pipeline files exist, collect all of them
- Handle cases where the pipeline folder doesn't exist or no match is found
  (return empty/not found)

YAML structure to parse (example):

```yaml
extends:
  pipeline: shared-templates/base-pipeline.yml # <-- extract this value
  parameters:
```

#### 3. Last Commit on Master/Default Branch

For each repo, fetch the latest commit on the default branch (master/main):
GET <https://dev.azure.com/{org}/{project}/_apis/git/repositories/{repoId}/commits>
?searchCriteria.itemVersion.version={defaultBranch}
&searchCriteria.$top=1
&api-version=7.0

Extract:

    Commit ID (short 8 chars)
    Author name
    Author email
    Commit date (formatted as YYYY-MM-DD HH:MM:SS)
    Commit message (first line only)

#### 4. Last Commit on Any Branch

For each repo, find the most recent commit across ALL branches:

    First, list all branches:
    GET https://dev.azure.com/{org}/{project}/_apis/git/repositories/{repoId}/refs
    ?filter=heads/
    &api-version=7.0
    For each branch, get the latest commit date from the ref's commit metadata
    Return the most recent one across all branches with:
        Branch name
        Author name
        Author email
        Commit date
        Commit message (first line only)

#### 5. Repository Activity Status

Determine if a repository is inactive using the following checks.
A repo is considered INACTIVE if ALL of the following are true:

    Last commit on ANY branch is older than INACTIVE_DAYS_THRESHOLD days
    Number of open pull requests is 0
    The repository is not disabled

Activity status levels:

    "ACTIVE" → last commit on any branch within threshold days
    "INACTIVE" → last commit on any branch older than threshold days
    "DORMANT" → no commits found at all
    "DISABLED" → repository is marked as disabled in Azure DevOps

Also calculate and include:

    Days since last commit on default branch
    Days since last commit on any branch
    Total number of branches
    Total number of open pull requests

Data Structures

Define the following Go structs:

```go

type PipelineInfo struct {
FileName string
ExtendsPipeline string // value of extends.pipeline in the YAML
}

type CommitInfo struct {
CommitID string
Author string
Email string
Date time.Time
Message string
BranchName string // only used for "last commit on any branch"
}

type RepoReport struct {
ProjectName string
RepoName string
RepoID string
DefaultBranch string
WebURL string
IsDisabled bool
TotalBranches int
OpenPRCount int
Pipelines []PipelineInfo
LastCommitDefault *CommitInfo
LastCommitAnyBranch*CommitInfo
DaysSinceDefaultCommit int
DaysSinceAnyCommit int
ActivityStatus string // ACTIVE, INACTIVE, DORMANT, DISABLED
}
```

HTTP Client Requirements

- Create a reusable HTTP client with:
  - Basic auth using PAT
  - 30-second timeout
  - Automatic retry (up to 3 times) on 429 (rate limit) and 5xx errors
  - Exponential backoff on retries
  - Respect the Retry-After header if present

Concurrency

- Process repositories concurrently using goroutines
- Use a worker pool pattern with a configurable number of workers (default 5, read from env var WORKER_COUNT)
- Use sync.WaitGroup and channels properly
- Protect shared data with sync.Mutex

Output

Generate TWO output formats based on a flag --output:

1. Console Table (default):
   Print a formatted table to stdout with columns:
   PROJECT | REPO | STATUS | DEFAULT BRANCH | LAST COMMIT (DEFAULT) | LAST COMMIT (ANY) | DAYS INACTIVE | BRANCHES | OPEN PRs | PIPELINE TEMPLATE

Color code the STATUS column:
ACTIVE → Green
INACTIVE → Yellow
DORMANT → Red
DISABLED → Gray

Use the github.com/fatih/color package for colors
Use the github.com/olekukonko/tablewriter package for table formatting

1. JSON file (when --output=json):
   Write the full []RepoReport slice as pretty-printed JSON to a file
   Default filename: azure-repos-report-{YYYY-MM-DD}.json

2. CSV file (when --output=csv):
   Write all fields to a CSV file
   Default filename: azure-repos-report-{YYYY-MM-DD}.csv

CLI Flags

Use the standard flag package:

- --output string Output format: table, json, csv (default: table)
- --project string Scan only a specific project (optional, default: all)
- --inactive-days int Days threshold for inactive status (default: 90)
- --workers int Number of concurrent workers (default: 5)
- --include-disabled bool Include disabled repos in output (default: false)
- --outfile string Output file path (for json/csv modes)

Error Handling

- Never crash on a single repo failure — log the error and continue
- Collect all errors and print a summary at the end
- Use structured logging with log/slog (Go 1.21+)

Project Structure

Organize into the following packages:

```
deadgit/
├── main.go
├── cmd/
│ └── cli.go // CLI flags and entry point
├── azuredevops/
│ ├── client.go // HTTP client, auth, retry logic
│ ├── projects.go // List projects
│ ├── repositories.go // List repos, get repo details
│ ├── commits.go // Get commits on branch, get commits across branches
│ ├── branches.go // List branches
│ ├── pullrequests.go // Count open PRs
│ └── items.go // Get file items, fetch file content
├── pipeline/
│ └── parser.go // YAML parsing for extends.pipeline
├── report/
│ ├── activity.go // Activity status calculation
│ ├── table.go // Console table output
│ ├── json.go // JSON output
│ └── csv.go // CSV output
└── go.mod
```

Go Module Dependencies

Use the following packages:

- gopkg.in/yaml.v3 → YAML parsing
- github.com/fatih/color → Terminal colors
- github.com/olekukonko/tablewriter → Table formatting
- Standard library only for everything else (net/http, encoding/json, etc.)

Additional Notes

- Use Go 1.21+
- All API calls must include the api-version=7.0 query parameter
- The PAT should be Base64 encoded as :PAT for Basic auth header
- Print a progress indicator while scanning (e.g., "Scanning repo X of Y...")
- At the end, print a summary:
  - Total projects scanned
  - Total repos scanned
  - Active / Inactive / Dormant / Disabled counts
  - Total scan duration

```

```
