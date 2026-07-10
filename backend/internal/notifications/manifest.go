package notifications

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jyates/jyatesdotdev-api/backend/internal/subscriptions"
)

const (
	manifestMaxBytes = 64 * 1024
	maxEvents        = 20
)

var manifestIDPattern = regexp.MustCompile(`^[a-f0-9]{7,64}$`)

type Manifest struct {
	Version int            `json:"version"`
	ID      string         `json:"id"`
	Events  []ContentEvent `json:"events"`
}

type ContentEvent struct {
	Topic   string `json:"topic"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	URL     string `json:"url"`
}

func ParseManifest(reader io.Reader) (Manifest, error) {
	limited := io.LimitReader(reader, manifestMaxBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return Manifest{}, err
	}
	if len(payload) > manifestMaxBytes {
		return Manifest{}, errors.New("notification manifest is too large")
	}

	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode notification manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("notification manifest contains trailing data")
	}
	if manifest.Version != 1 || !manifestIDPattern.MatchString(manifest.ID) {
		return Manifest{}, errors.New("invalid notification manifest metadata")
	}
	if len(manifest.Events) == 0 || len(manifest.Events) > maxEvents {
		return Manifest{}, errors.New("invalid notification event count")
	}
	for index := range manifest.Events {
		manifest.Events[index].Title = strings.TrimSpace(manifest.Events[index].Title)
		manifest.Events[index].Summary = strings.TrimSpace(manifest.Events[index].Summary)
		manifest.Events[index].URL = strings.TrimSpace(manifest.Events[index].URL)
		event := manifest.Events[index]
		if err := validateEvent(event); err != nil {
			return Manifest{}, fmt.Errorf("event %d: %w", index, err)
		}
	}
	return manifest, nil
}

func validateEvent(event ContentEvent) error {
	if event.Topic != subscriptions.TopicBlog && event.Topic != subscriptions.TopicProjects {
		return errors.New("unsupported topic")
	}
	if !validText(event.Title, 200) || strings.ContainsAny(event.Title, "\r\n") ||
		!validText(event.Summary, 2000) {
		return errors.New("invalid title or summary")
	}
	parsed, err := url.Parse(event.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "jyates.dev" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("invalid content URL")
	}
	if event.Topic == subscriptions.TopicBlog && !strings.HasPrefix(parsed.Path, "/blog/") {
		return errors.New("blog notification URL must target a post")
	}
	if event.Topic == subscriptions.TopicProjects && parsed.Path != "/projects" {
		return errors.New("project notification URL must target the projects page")
	}
	return nil
}

func validText(value string, maxRunes int) bool {
	value = strings.TrimSpace(value)
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxRunes
}

func (event ContentEvent) Subject() string {
	prefix := "New project: "
	if event.Topic == subscriptions.TopicBlog {
		prefix = "New blog post: "
	}
	return prefix + event.Title
}

func (event ContentEvent) Body() string {
	return fmt.Sprintf("%s\n\n%s\n\nRead more: %s", event.Title, event.Summary, event.URL)
}
