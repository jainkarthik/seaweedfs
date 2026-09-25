package s3api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A marker that sorts before the prefix excludes nothing under the prefix, so the
// listing must be the one with no marker. docker/distribution's S3 driver sends
// exactly this shape: prefix "<root>/<path>/" with start-after "<root>".
func Test_markerSortsBeforePrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		marker string
		want   bool
	}{
		{"marker is the root directory the prefix walks under", "docker/registry/", "docker", true},
		{"marker is the prefix directory without its slash", "docker/registry/", "docker/registry", true},
		{"marker is an unrelated earlier key", "docker/registry/", "a", true},
		{"marker is an earlier sibling", "docker/registry/", "docker/regisr", true},
		{"leading slashes are ignored on both sides", "/docker/registry/", "/docker", true},
		{"empty marker is not a cutoff", "docker/registry/", "", false},
		{"empty prefix: every key is in scope, marker stands", "", "docker", false},
		{"marker under the prefix resumes inside it", "docker/registry/", "docker/registry/v2/link", false},
		{"marker equal to the prefix stands", "docker/registry/", "docker/registry/", false},
		{"marker after the prefix's subtree stands", "docker/registry/", "docker/registryx", false},
		{"partial name prefix: a later match-set marker stands", "parent", "parentDir/data/0e", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, markerSortsBeforePrefix(tt.prefix, tt.marker), "markerSortsBeforePrefix(%q, %q)", tt.prefix, tt.marker)
		})
	}
}

// A marker ending on the delimiter is trimmed to a shorter cutoff for the walk, which no
// longer excludes the key the client named, so that key is skipped as it streams.
func Test_excludedMarkerKey(t *testing.T) {
	tests := []struct {
		name          string
		requestMarker string
		marker        string
		want          string
	}{
		{"marker trimmed to a shorter cutoff", "docker/", "docker", "docker/"},
		{"leading slashes are dropped, as the keys carry none", "/docker/", "docker", "docker/"},
		{"untrimmed marker: the walk already excludes it", "docker", "docker", ""},
		{"no marker", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, excludedMarkerKey(tt.requestMarker, tt.marker), "excludedMarkerKey(%q, %q)", tt.requestMarker, tt.marker)
		})
	}
}
