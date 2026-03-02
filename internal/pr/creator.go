package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

// CommandRunner abstracts os/exec for testability.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	RunInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

// PRCreator automates PR creation for right-sizing recommendations.
type PRCreator struct {
	runner    CommandRunner
	serverURL string
}

// NewPRCreator creates a PRCreator with the given command runner and server URL.
func NewPRCreator(runner CommandRunner, serverURL string) *PRCreator {
	return &PRCreator{runner: runner, serverURL: serverURL}
}

// PRResult holds the output of a PR creation flow.
type PRResult struct {
	PRURL       string
	Branch      string
	Title       string
	Body        string
	FilePath    string
	DryRun      bool
	Recommendation *models.Recommendation
}

// Annotations holds sage GitOps annotations from a workload.
type Annotations struct {
	Repo  string
	Path  string
	Field string
}

// Options configures the PR creation flow.
type Options struct {
	Kind         string
	Name         string
	Namespace    string
	BranchPrefix string
	DryRun       bool
}

// Create runs the full PR creation flow.
func (p *PRCreator) Create(ctx context.Context, opts Options) (*PRResult, error) {
	// Step 1: Check gh auth status.
	if err := p.checkGHAuth(ctx); err != nil {
		return nil, err
	}

	// Step 2: Read annotations from the target workload.
	annotations, err := p.readAnnotations(ctx, opts.Kind, opts.Name, opts.Namespace)
	if err != nil {
		return nil, fmt.Errorf("reading annotations: %w", err)
	}

	// Step 3: Fetch recommendation from server API.
	rec, err := p.fetchRecommendation(ctx, opts.Namespace, opts.Kind, opts.Name)
	if err != nil {
		return nil, fmt.Errorf("fetching recommendation: %w", err)
	}

	title := fmt.Sprintf("chore: right-size %s/%s resources", strings.ToLower(opts.Kind), opts.Name)
	body := FormatPRBody(rec, opts.Kind, opts.Name, opts.Namespace)
	branch := FormatBranchName(opts.BranchPrefix, opts.Namespace, opts.Name)

	result := &PRResult{
		Branch:         branch,
		Title:          title,
		Body:           body,
		FilePath:       annotations.Path,
		DryRun:         opts.DryRun,
		Recommendation: rec,
	}

	if opts.DryRun {
		return result, nil
	}

	// Step 5: Clone repo to temp dir.
	tmpDir, err := os.MkdirTemp("", "sage-pr-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := filepath.Join(tmpDir, "repo")
	if _, err := p.runner.Run(ctx, "gh", "repo", "clone", annotations.Repo, repoDir); err != nil {
		return nil, fmt.Errorf("cloning repo %s: %w", annotations.Repo, err)
	}

	// Step 6: Create branch.
	if _, err := p.runner.RunInDir(ctx, repoDir, "git", "checkout", "-b", branch); err != nil {
		return nil, fmt.Errorf("creating branch %s: %w", branch, err)
	}

	// Step 7: Modify YAML file.
	filePath := filepath.Join(repoDir, annotations.Path)
	// Prevent path traversal: ensure resolved path stays within the cloned repo.
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolving file path: %w", err)
	}
	absRepoDir, err := filepath.Abs(repoDir)
	if err != nil {
		return nil, fmt.Errorf("resolving repo dir: %w", err)
	}
	if !strings.HasPrefix(absFilePath, absRepoDir+string(filepath.Separator)) {
		return nil, fmt.Errorf("path %q escapes repo directory", annotations.Path)
	}
	if err := p.modifyResourceFile(filePath, annotations, rec); err != nil {
		return nil, fmt.Errorf("modifying %s: %w", annotations.Path, err)
	}

	// Step 8: Commit.
	commitMsg := fmt.Sprintf("chore: right-size %s/%s resources", strings.ToLower(opts.Kind), opts.Name)
	if _, err := p.runner.RunInDir(ctx, repoDir, "git", "add", annotations.Path); err != nil {
		return nil, fmt.Errorf("git add: %w", err)
	}
	if _, err := p.runner.RunInDir(ctx, repoDir, "git", "commit", "-m", commitMsg); err != nil {
		return nil, fmt.Errorf("git commit: %w", err)
	}

	// Step 9: Push.
	if _, err := p.runner.RunInDir(ctx, repoDir, "git", "push", "-u", "origin", branch); err != nil {
		return nil, fmt.Errorf("git push: %w", err)
	}

	// Step 10: Create PR.
	prOut, err := p.runner.RunInDir(ctx, repoDir, "gh", "pr", "create",
		"--title", title,
		"--body", body)
	if err != nil {
		return nil, fmt.Errorf("creating PR: %w", err)
	}

	result.PRURL = strings.TrimSpace(string(prOut))
	return result, nil
}

// checkGHAuth verifies gh CLI is authenticated.
func (p *PRCreator) checkGHAuth(ctx context.Context) error {
	_, err := p.runner.Run(ctx, "gh", "auth", "status")
	if err != nil {
		return fmt.Errorf("gh CLI not authenticated. Run 'gh auth login' first: %w", err)
	}
	return nil
}

// readAnnotations reads sage.io/* annotations from the target workload.
func (p *PRCreator) readAnnotations(ctx context.Context, kind, name, namespace string) (*Annotations, error) {
	target := strings.ToLower(kind) + "/" + name
	args := []string{"get", target, "-o", "jsonpath={.metadata.annotations}"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	out, err := p.runner.Run(ctx, "kubectl", args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl get %s: %w", target, err)
	}

	var raw map[string]string
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing annotations: %w", err)
	}

	repo := raw["sage.io/repo"]
	path := raw["sage.io/path"]
	if repo == "" || path == "" {
		return nil, fmt.Errorf("missing required annotations sage.io/repo and sage.io/path on %s", target)
	}

	return &Annotations{
		Repo:  repo,
		Path:  path,
		Field: raw["sage.io/field"],
	}, nil
}

// fetchRecommendation gets the recommendation for the target workload from the server.
func (p *PRCreator) fetchRecommendation(ctx context.Context, namespace, kind, name string) (*models.Recommendation, error) {
	q := url.Values{}
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	reqURL := p.serverURL + "/api/v1/recommendations"
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	const maxResponseBytes = 10 * 1024 * 1024 // 10MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var envelope models.APIResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if envelope.Status == "error" {
		return nil, fmt.Errorf("server error: %s", envelope.Error)
	}

	raw, err := json.Marshal(envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("re-marshalling data: %w", err)
	}

	var recs []models.Recommendation
	if err := json.Unmarshal(raw, &recs); err != nil {
		return nil, fmt.Errorf("decoding recommendations: %w", err)
	}

	// Filter to the target workload — find first CPU recommendation.
	lowerKind := strings.ToLower(kind)
	for i := range recs {
		if strings.ToLower(recs[i].Target.Kind) == lowerKind &&
			recs[i].Target.Name == name &&
			recs[i].Resource == "cpu" {
			return &recs[i], nil
		}
	}
	// Fallback: find any recommendation for this target.
	for i := range recs {
		if strings.ToLower(recs[i].Target.Kind) == lowerKind &&
			recs[i].Target.Name == name {
			return &recs[i], nil
		}
	}

	return nil, fmt.Errorf("no recommendation found for %s/%s in namespace %s", kind, name, namespace)
}

// modifyResourceFile updates resource requests/limits in the specified file.
func (p *PRCreator) modifyResourceFile(filePath string, annotations *Annotations, rec *models.Recommendation) error {
	content, err := os.ReadFile(filePath) // #nosec G304 — path validated at call site (traversal check)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	modified := string(content)
	isValuesFile := strings.HasSuffix(filePath, "values.yaml") || strings.HasSuffix(filePath, "values.yml")

	if isValuesFile && annotations.Field != "" {
		modified, err = modifyValuesFile(modified, annotations.Field, rec)
	} else {
		modified, err = modifyManifestFile(modified, rec)
	}

	if err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(modified), 0600) // #nosec G304 — path validated at call site
}

// modifyManifestFile updates CPU/memory values in a standard K8s manifest.
// Known limitations: assumes 2-space indentation, single container per manifest,
// and standard requests:/limits: block structure. Multi-container pods or
// non-standard formatting may require manual adjustment after PR creation.
func modifyManifestFile(content string, rec *models.Recommendation) (string, error) {
	result := content

	if rec.Resource == "cpu" {
		result = replaceCPUValue(result, "requests", formatCPU(rec.RecommendedReq))
		if rec.RecommendedLimit > 0 {
			result = replaceCPUValue(result, "limits", formatCPU(rec.RecommendedLimit))
		}
	} else if rec.Resource == "memory" {
		result = replaceMemoryValue(result, "requests", formatMemory(rec.RecommendedReq))
		if rec.RecommendedLimit > 0 {
			result = replaceMemoryValue(result, "limits", formatMemory(rec.RecommendedLimit))
		}
	}

	if result == content {
		return content, fmt.Errorf("no resource values found to modify")
	}

	return result, nil
}

// modifyValuesFile updates resources in a Helm values.yaml. The fieldPath annotation
// guides which section to target but the current implementation uses the same
// line-based replacement as modifyManifestFile. Known limitations: assumes a flat
// resources block with 2-space indentation. Deeply nested or templated values files
// may require manual adjustment after PR creation.
func modifyValuesFile(content string, fieldPath string, rec *models.Recommendation) (string, error) {
	result := content

	if rec.Resource == "cpu" {
		result = replaceCPUValue(result, "requests", formatCPU(rec.RecommendedReq))
		if rec.RecommendedLimit > 0 {
			result = replaceCPUValue(result, "limits", formatCPU(rec.RecommendedLimit))
		}
	} else if rec.Resource == "memory" {
		result = replaceMemoryValue(result, "requests", formatMemory(rec.RecommendedReq))
		if rec.RecommendedLimit > 0 {
			result = replaceMemoryValue(result, "limits", formatMemory(rec.RecommendedLimit))
		}
	}

	if result == content {
		return content, fmt.Errorf("no resource values found to modify in values file")
	}

	return result, nil
}

// cpuPattern matches cpu: <value> lines within a requests: or limits: block.
var cpuPattern = regexp.MustCompile(`(cpu:\s*)["']?(\d+m?)["']?`)

// memPattern matches memory: <value> lines within a requests: or limits: block.
var memPattern = regexp.MustCompile(`(memory:\s*)["']?(\d+[KMGT]i?)["']?`)

// replaceCPUValue replaces CPU values in the content. The section parameter
// (requests/limits) is used for context but we replace all matching cpu: lines.
func replaceCPUValue(content, section, newValue string) string {
	// Find the section (requests: or limits:) and replace cpu value after it.
	lines := strings.Split(content, "\n")
	inSection := false
	modified := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == section+":" || strings.HasPrefix(trimmed, section+":") {
			inSection = true
			continue
		}
		// Detect leaving the section (next key at same or lower indent).
		if inSection && len(trimmed) > 0 && !strings.HasPrefix(line, "  ") && trimmed != "" {
			if !strings.HasPrefix(trimmed, "cpu:") && !strings.HasPrefix(trimmed, "memory:") {
				inSection = false
			}
		}
		if inSection && cpuPattern.MatchString(trimmed) {
			lines[i] = cpuPattern.ReplaceAllString(line, "${1}"+newValue)
			modified = true
			inSection = false
		}
	}

	if modified {
		return strings.Join(lines, "\n")
	}
	return content
}

// replaceMemoryValue replaces memory values in the content.
func replaceMemoryValue(content, section, newValue string) string {
	lines := strings.Split(content, "\n")
	inSection := false
	modified := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == section+":" || strings.HasPrefix(trimmed, section+":") {
			inSection = true
			continue
		}
		if inSection && len(trimmed) > 0 && !strings.HasPrefix(line, "  ") && trimmed != "" {
			if !strings.HasPrefix(trimmed, "cpu:") && !strings.HasPrefix(trimmed, "memory:") {
				inSection = false
			}
		}
		if inSection && memPattern.MatchString(trimmed) {
			lines[i] = memPattern.ReplaceAllString(line, "${1}"+newValue)
			modified = true
			inSection = false
		}
	}

	if modified {
		return strings.Join(lines, "\n")
	}
	return content
}

// formatCPU formats millicores as a K8s CPU string.
func formatCPU(millis int64) string {
	if millis%1000 == 0 {
		return fmt.Sprintf("%d", millis/1000)
	}
	return fmt.Sprintf("%dm", millis)
}

// formatMemory formats bytes as a K8s memory string.
func formatMemory(bytes int64) string {
	gi := int64(1024 * 1024 * 1024)
	mi := int64(1024 * 1024)

	if bytes >= gi && bytes%gi == 0 {
		return fmt.Sprintf("%dGi", bytes/gi)
	}
	if bytes >= mi {
		return fmt.Sprintf("%dMi", bytes/mi)
	}
	return fmt.Sprintf("%d", bytes)
}

// FormatBranchName generates a branch name from the prefix, namespace, and workload name.
func FormatBranchName(prefix, namespace, name string) string {
	ts := time.Now().Format("20060102-150405")
	return fmt.Sprintf("%s/%s-%s-%s", prefix, namespace, name, ts)
}

// FormatPRBody generates the PR body with recommendation details.
func FormatPRBody(rec *models.Recommendation, kind, name, namespace string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Right-sizing: %s/%s\n\n", kind, name))
	b.WriteString(fmt.Sprintf("**Namespace:** %s\n", namespace))
	b.WriteString(fmt.Sprintf("**Pattern:** %s\n", rec.Pattern))
	b.WriteString(fmt.Sprintf("**Confidence:** %.0f%%\n", rec.Confidence*100))
	b.WriteString(fmt.Sprintf("**Risk:** %s\n\n", rec.Risk))

	b.WriteString("### Changes\n\n")
	b.WriteString("| Resource | Current | Recommended |\n")
	b.WriteString("|----------|---------|-------------|\n")

	if rec.Resource == "cpu" {
		b.WriteString(fmt.Sprintf("| CPU request | %s | %s |\n", formatCPU(rec.CurrentRequest), formatCPU(rec.RecommendedReq)))
		if rec.RecommendedLimit > 0 {
			b.WriteString(fmt.Sprintf("| CPU limit | %s | %s |\n", formatCPU(rec.CurrentLimit), formatCPU(rec.RecommendedLimit)))
		}
	} else {
		b.WriteString(fmt.Sprintf("| Memory request | %s | %s |\n", formatMemory(rec.CurrentRequest), formatMemory(rec.RecommendedReq)))
		if rec.RecommendedLimit > 0 {
			b.WriteString(fmt.Sprintf("| Memory limit | %s | %s |\n", formatMemory(rec.CurrentLimit), formatMemory(rec.RecommendedLimit)))
		}
	}

	b.WriteString(fmt.Sprintf("\n**Reasoning:** %s\n", rec.Reasoning))
	b.WriteString("\n---\nGenerated by [k8s-sage](https://github.com/gregorytcarroll/k8s-sage)\n")

	return b.String()
}
