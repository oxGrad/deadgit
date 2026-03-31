package azuredevops

import "time"

// Projects API
type ProjectList struct {
	Value []Project `json:"value"`
	Count int       `json:"count"`
}

type Project struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Repositories API
type RepositoryList struct {
	Value []Repository `json:"value"`
	Count int          `json:"count"`
}

type Repository struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	DefaultBranch string  `json:"defaultBranch"`
	RemoteURL     string  `json:"remoteUrl"`
	WebURL        string  `json:"webUrl"`
	IsDisabled    bool    `json:"isDisabled"`
	Project       Project `json:"project"`
}

// Branches (refs) API
type RefList struct {
	Value []Ref `json:"value"`
	Count int   `json:"count"`
}

type Ref struct {
	Name     string `json:"name"`
	ObjectID string `json:"objectId"`
}

// Commits API
type CommitList struct {
	Value []Commit `json:"value"`
	Count int      `json:"count"`
}

type Commit struct {
	CommitID  string    `json:"commitId"`
	Author    GitPerson `json:"author"`
	Committer GitPerson `json:"committer"`
	Comment   string    `json:"comment"`
}

type GitPerson struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// Pull Requests API
type PullRequestList struct {
	Value []PullRequest `json:"value"`
	Count int           `json:"count"`
}

type PullRequest struct {
	PullRequestID int    `json:"pullRequestId"`
	Title         string `json:"title"`
	Status        string `json:"status"`
}

// Items API
type ItemList struct {
	Value []Item `json:"value"`
	Count int    `json:"count"`
}

type Item struct {
	ObjectID      string `json:"objectId"`
	GitObjectType string `json:"gitObjectType"`
	CommitID      string `json:"commitId"`
	Path          string `json:"path"`
	URL           string `json:"url"`
}
