package knowledge_test

// Static half of the zero-token guarantee: the packages that extract rules
// and generate the skill cannot reach the network, spawn processes, or link
// any LLM / embedding SDK. The runtime half is
// pipeline.TestZeroTokenSkillGeneration.

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Packages that our own extraction / generation code may never import.
var forbiddenDirect = []string{"net", "net/http", "net/rpc", "net/smtp", "crypto/tls", "os/exec", "plugin"}

// Module path fragments of AI/LLM/embedding SDKs.
var forbiddenModules = []string{
	"openai", "anthropic", "generative-ai", "genai", "vertexai", "cohere", "langchain", "ollama",
	"mistral", "huggingface", "replicate", "llama", "gemini", "bedrockruntime", "embedding",
}

// Network client packages that must not be reachable even transitively.
var forbiddenTransitive = []string{"net/http", "crypto/tls", "net/rpc", "net/smtp", "google.golang.org/grpc"}

func goList(t *testing.T, args ...string) []string {
	t.Helper()
	gobin := filepath.Join(runtime.GOROOT(), "bin", "go")
	out, err := exec.Command(gobin, append([]string{"list"}, args...)...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list %v: %s", args, ee.Stderr)
		}
		t.Fatalf("go list %v: %v", args, err)
	}
	return strings.Fields(string(out))
}

func TestGeneratorPackagesHaveNoNetworkOrExecImports(t *testing.T) {
	for _, pkg := range []string{"db", "extract", "skillgen"} {
		imports := goList(t, "-f", `{{join .Imports " "}}`, "review-dock/backend/knowledge/"+pkg)
		for _, imp := range imports {
			for _, bad := range forbiddenDirect {
				if imp == bad {
					t.Errorf("backend/knowledge/%s imports %s", pkg, imp)
				}
			}
		}
	}
}

func TestSkillRendererIsStdlibOnly(t *testing.T) {
	// skillgen turns the snapshot into Markdown. Its whole dependency tree is
	// the Go standard library minus networking and subprocesses.
	deps := goList(t, "-deps", "-f", `{{if not .Standard}}{{.ImportPath}}{{end}}`, "review-dock/backend/knowledge/skillgen")
	for _, d := range deps {
		if d != "review-dock/backend/knowledge/skillgen" {
			t.Errorf("skillgen depends on non-stdlib package %s", d)
		}
	}
	all := goList(t, "-deps", "review-dock/backend/knowledge/skillgen")
	for _, d := range all {
		for _, bad := range forbiddenDirect {
			if d == bad {
				t.Errorf("skillgen transitively depends on %s", d)
			}
		}
	}
}

func TestNoNetworkClientOrAISDKReachable(t *testing.T) {
	for _, pkg := range []string{"db", "extract", "skillgen"} {
		for _, d := range goList(t, "-deps", "review-dock/backend/knowledge/"+pkg) {
			for _, bad := range forbiddenTransitive {
				if d == bad || strings.HasPrefix(d, bad+"/") {
					t.Errorf("%s transitively reaches network client %s", pkg, d)
				}
			}
			lower := strings.ToLower(d)
			for _, m := range forbiddenModules {
				if strings.Contains(lower, m) {
					t.Errorf("%s transitively depends on AI module %s", pkg, d)
				}
			}
		}
	}
}

func TestWholeModuleLinksNoAISDK(t *testing.T) {
	mods := goList(t, "-m", "all")
	for _, m := range mods {
		lower := strings.ToLower(m)
		for _, bad := range forbiddenModules {
			if strings.Contains(lower, bad) {
				t.Errorf("go.mod requires AI/LLM module %s", m)
			}
		}
	}
}
