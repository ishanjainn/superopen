package scope

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultTenant = "local"

// Scope is who owns a row and which repo it belongs to.
type Scope struct {
	TenantID    string
	PrincipalID string
	ProjectID   string
}

// Current reads the process tenant and principal and the repo's project id.
// An empty SUPEROPEN_TENANT means the single-machine tenant.
func Current(repoRoot string) (Scope, error) {
	tenant := strings.TrimSpace(os.Getenv("SUPEROPEN_TENANT"))
	if tenant == "" {
		tenant = DefaultTenant
	}
	return Scope{
		TenantID:    tenant,
		PrincipalID: Principal(),
		ProjectID:   ProjectID(repoRoot),
	}.Validated()
}

// Principal is who owns personal rows in this process.
// An explicit user wins. Otherwise the OS username. A machine with no
// username, such as a container running as a numeric uid, uses that uid.
func Principal() string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	return resolvePrincipal(os.Getenv("SUPEROPEN_USER"), os.Getenv("SO_USER"), name, os.Getuid())
}

func resolvePrincipal(envUser, soUser, username string, uid int) string {
	if p := strings.TrimSpace(envUser); p != "" {
		return p
	}
	if p := strings.TrimSpace(soUser); p != "" {
		return p
	}
	if p := strings.TrimSpace(username); p != "" {
		return p
	}
	if uid >= 0 {
		return strconv.Itoa(uid)
	}
	return ""
}

// ProjectID is the stable id of a checkout.
func ProjectID(repoRoot string) string {
	clean := filepath.Clean(strings.TrimSpace(repoRoot))
	if clean == "" || clean == "." {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil && resolved != "" {
		clean = resolved
	}
	sum := sha1.Sum([]byte(clean))
	return hex.EncodeToString(sum[:8])
}

// Validated returns an error when any id is empty.
func (s Scope) Validated() (Scope, error) {
	s.TenantID = strings.TrimSpace(s.TenantID)
	s.PrincipalID = strings.TrimSpace(s.PrincipalID)
	s.ProjectID = strings.TrimSpace(s.ProjectID)
	if s.TenantID == "" || s.PrincipalID == "" || s.ProjectID == "" {
		return Scope{}, fmt.Errorf("scope requires tenant, principal, and project")
	}
	return s, nil
}

// Check rejects a scope that cannot be used in a query.
func Check(s Scope) error {
	_, err := s.Validated()
	return err
}

// SQLLiteral quotes a value for a predicate that must travel with the query.
func SQLLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
