package notifications

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest_AcceptsTrustedContentMetadata(t *testing.T) {
	manifest, err := ParseManifest(strings.NewReader(`{
  "version": 1,
  "id": "0123456789abcdef0123456789abcdef01234567",
  "events": [{
    "topic": "blog",
    "title": "A new post",
    "summary": "What changed and why.",
    "url": "https://jyates.dev/blog/a-new-post"
  }]
}`))

	require.NoError(t, err)
	assert.Equal(t, "New blog post: A new post", manifest.Events[0].Subject())
	assert.Contains(t, manifest.Events[0].Body(), "https://jyates.dev/blog/a-new-post")
}

func TestParseManifest_RejectsExternalURLs(t *testing.T) {
	_, err := ParseManifest(strings.NewReader(`{
  "version": 1,
  "id": "0123456789abcdef0123456789abcdef01234567",
  "events": [{
    "topic": "projects",
    "title": "A new project",
    "summary": "Summary",
    "url": "https://example.com/phishing"
  }]
}`))

	assert.ErrorContains(t, err, "invalid content URL")
}

func TestParseManifest_RejectsUnknownFields(t *testing.T) {
	_, err := ParseManifest(strings.NewReader(`{
  "version": 1,
  "id": "0123456789abcdef0123456789abcdef01234567",
  "unexpected": true,
  "events": []
}`))

	assert.ErrorContains(t, err, "unknown field")
}
