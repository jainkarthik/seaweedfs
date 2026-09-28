package s3api

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/seaweedfs/seaweedfs/weed/s3api/s3_constants"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleXMLParsingWithoutNamespace(t *testing.T) {
	xmlData := `<LifecycleConfiguration><Rule><ID>expire-rule</ID><Status>Enabled</Status><Filter><Prefix>tmp/</Prefix></Filter><Expiration><Days>1</Days></Expiration></Rule></LifecycleConfiguration>`

	var lc Lifecycle
	err := xml.Unmarshal([]byte(xmlData), &lc)
	require.NoError(t, err, "Should successfully parse LifecycleConfiguration without xmlns")
	require.Len(t, lc.Rules, 1)
	assert.Equal(t, "expire-rule", lc.Rules[0].ID)
	assert.Equal(t, Enabled, lc.Rules[0].Status)
	assert.Equal(t, "tmp/", lc.Rules[0].Filter.Prefix.val)
	assert.Equal(t, 1, lc.Rules[0].Expiration.Days)
}

func TestLifecycleXMLParsingWithNamespace(t *testing.T) {
	xmlData := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>expire-rule</ID><Status>Enabled</Status><Filter><Prefix>tmp/</Prefix></Filter><Expiration><Days>1</Days></Expiration></Rule></LifecycleConfiguration>`

	var lc Lifecycle
	err := xml.Unmarshal([]byte(xmlData), &lc)
	require.NoError(t, err, "Should successfully parse LifecycleConfiguration with xmlns")
	require.Len(t, lc.Rules, 1)
	assert.Equal(t, "expire-rule", lc.Rules[0].ID)
	assert.Equal(t, Enabled, lc.Rules[0].Status)
	assert.Equal(t, "tmp/", lc.Rules[0].Filter.Prefix.val)
	assert.Equal(t, 1, lc.Rules[0].Expiration.Days)
}

func TestLifecycleXMLMarshalingWithNamespace(t *testing.T) {
	lc := Lifecycle{
		Xmlns: s3_constants.S3Namespace,
		Rules: []Rule{
			{
				ID:         "rule-1",
				Status:     Enabled,
				Prefix:     Prefix{val: "logs/", set: true},
				Expiration: Expiration{Days: 7, set: true},
			},
		},
	}

	encoded := string(s3err.EncodeXMLResponse(lc))
	assert.True(t, strings.Contains(encoded, `xmlns="http://s3.amazonaws.com/doc/2006-03-01/"`), "Encoded XML must contain canonical S3 namespace: %s", encoded)
	assert.True(t, strings.Contains(encoded, `<ID>rule-1</ID>`), "Encoded XML must contain rule ID: %s", encoded)
}

func TestLifecycleXMLFilterPrefixPreservation(t *testing.T) {
	xmlData := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>bryck-compat</ID><Filter><Prefix>compatibility/2026-09-27_233104/</Prefix></Filter><Status>Enabled</Status><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`

	var lc Lifecycle
	err := xml.Unmarshal([]byte(xmlData), &lc)
	require.NoError(t, err)
	require.Len(t, lc.Rules, 1)
	assert.Equal(t, "bryck-compat", lc.Rules[0].ID)
	assert.Equal(t, Enabled, lc.Rules[0].Status)
	assert.Equal(t, "compatibility/2026-09-27_233104/", lc.Rules[0].Filter.Prefix.val)
	assert.Equal(t, 30, lc.Rules[0].Expiration.Days)

	// Verify default transition minimum object size
	headerVal := normalizeBucketLifecycleTransitionMinimumObjectSize("")
	assert.Equal(t, "all_storage_classes_128K", headerVal)
}
