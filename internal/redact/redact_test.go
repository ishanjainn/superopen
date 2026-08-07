package redact

import (
	"strings"
	"testing"
)

// Vendor prefixes that GitHub's push-protection scanner shape-matches
// against (Stripe `sk_live_…`, etc.) are assembled at runtime via
// `strings.Repeat` so the source file never contains
// "<prefix><20+ alphanumerics>" as a single literal. The redactor still
// sees the full, well-formed fake key when the test runs because Go
// string concatenation produces the same bytes the regex expects -
// the scanner just can't see it on disk. Don't inline these.
var (
	stripeSkLiveFake = "sk_live_" + strings.Repeat("a", 30)
	stripeRkLiveFake = "rk_live_" + strings.Repeat("a", 30)
	awsAccessKeyFake = "AKIA" + strings.Repeat("A", 16)
	awsSecretKeyFake = `aws_secret_access_key="` + strings.Repeat("a", 40) + `"`
	ghPatFake        = "ghp_" + strings.Repeat("a", 36)
	openaiSkFake     = "sk-proj-" + strings.Repeat("a", 10) + "-secrets-living-here-now"
	anthropicSkFake  = "sk-ant-" + strings.Repeat("a", 6) + "-secrets-456-zzz"
	googleAPIKeyFake = "AIza" + strings.Repeat("0", 35)
	slackXoxbFake    = "xoxb-12345678-1234567890123-123456789012-" + strings.Repeat("a", 20)
	pemHeader        = strings.Join([]string{"-----BEGIN", "RSA", "PRIVATE", "KEY-----"}, " ")
	pemFooter        = strings.Join([]string{"-----END", "RSA", "PRIVATE", "KEY-----"}, " ")
	privateKeyFake   = pemHeader + "\nMIIEvQIBADANBgkq\n" + pemFooter
	azureSasFake     = "?sv=2021-08-06&sig=" + strings.Repeat("A", 20) + "%3D"
	azureStorageFake = "DefaultEndpointsProtocol=https;AccountName=foo;AccountKey=" + strings.Repeat("A", 20) + "=="
	hfTokenFake      = "hf_" + strings.Repeat("A", 25)
	npmTokenFake     = "npm_" + strings.Repeat("a", 33)
)

// shouldRedactTier1 lists strings that contain a secret tier-1 should
// remove. The check is "Replacement appears in the output" rather than
// exact equality, since some patterns leave a prefix in place
// (e.g. "Authorization: Bearer ...").
var shouldRedactTier1 = []struct {
	name string
	in   string
}{
	{"aws_access_key_id", awsAccessKeyFake + " inside a sentence"},
	{"aws_secret_access_key_assignment", awsSecretKeyFake},
	{"gh_pat", ghPatFake},
	{"openai_sk", openaiSkFake},
	{"anthropic_sk", anthropicSkFake},
	{"google_api_key", googleAPIKeyFake},
	{"slack_xoxb", slackXoxbFake},
	{"stripe_sk_live", stripeSkLiveFake},
	{"stripe_rk_live", stripeRkLiveFake},
	{"jwt", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0.signature123"},
	{"bearer_header", "Authorization: Bearer abc123def456ghi789jkl"},
	{"private_key", privateKeyFake},
	{"azure_sas_sig", azureSasFake},
	{"azure_storage_conn", azureStorageFake},
	{"hf_token", hfTokenFake},
	{"npm_token", npmTokenFake},
	{"postgres_url", "postgres://app:" + strings.Repeat("s", 12) + "@db.internal:5432/main"},
	{"mysql_url", "mysql://root:" + strings.Repeat("h", 14) + "@10.0.0.5/orders"},
}

func TestStringTier1RedactsKnownSecrets(t *testing.T) {
	for _, tc := range shouldRedactTier1 {
		t.Run(tc.name, func(t *testing.T) {
			out := String(tc.in)
			if !strings.Contains(out, Replacement) {
				t.Errorf("expected replacement marker; got %q", out)
			}
		})
	}
}

func TestPostgresURLKeepsHostDropsCreds(t *testing.T) {
	// The capture-rewrite path must keep the scheme + host so a
	// dashboard can still tell which DB the agent was hitting, while
	// dropping the user + password. Asserts the exact shape so a
	// future refactor of the replacement template can't silently
	// regress (e.g. swallowing the path).
	in := "postgres://app:s3cret-Pa55@db.internal:5432/main"
	want := "postgres://[REDACTED]:[REDACTED]@db.internal:5432/main"
	if got := String(in); got != want {
		t.Errorf("String(%q) = %q, want %q", in, got, want)
	}
}

func TestStringTier1LeavesNonSecretsAlone(t *testing.T) {
	cases := []string{
		"hello world",
		"User clicked the merge button",
		"no secrets here, just an explanation about authentication",
		"sk-",   // too short to match
		"AKIA1", // too short to match
	}
	for _, s := range cases {
		if got := String(s); got != s {
			t.Errorf("String(%q) = %q (changed unexpectedly)", s, got)
		}
	}
}

func TestStringFullCatchesGenericPasswords(t *testing.T) {
	cases := []string{
		`password="hunter22hunter22"`,
		`password=hunter22hunter22`,
		`api_key: "sk-some-thing-12345-678"`,
	}
	for _, s := range cases {
		out := StringFull(s)
		if !strings.Contains(out, Replacement) {
			t.Errorf("StringFull(%q) did not redact: got %q", s, out)
		}
	}
}

func TestForCaptureSelector(t *testing.T) {
	if got := ForCapture("metadata_only")("password=secret_value_123"); got == "password="+Replacement {
		// Tier 1 alone should NOT match this generic password pattern.
		t.Errorf("metadata_only mode unexpectedly applied tier 2: %q", got)
	}
	if got := ForCapture("full")("password=secret_value_123"); !strings.Contains(got, Replacement) {
		t.Errorf("full mode should have redacted: %q", got)
	}
}

func TestEmptyString(t *testing.T) {
	if got := String(""); got != "" {
		t.Errorf("String(%q) = %q", "", got)
	}
	if got := StringFull(""); got != "" {
		t.Errorf("StringFull(%q) = %q", "", got)
	}
}
