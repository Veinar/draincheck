package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type krewManifest struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Version          string `json:"version"`
		Homepage         string `json:"homepage"`
		ShortDescription string `json:"shortDescription"`
		Description      string `json:"description"`
		Platforms        []struct {
			Selector struct {
				MatchLabels map[string]string `json:"matchLabels"`
			} `json:"selector"`
			URI    string `json:"uri"`
			Sha256 string `json:"sha256"`
			Bin    string `json:"bin"`
			Files  []struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"files"`
		} `json:"platforms"`
	} `json:"spec"`
}

type goreleaserConfig struct {
	ProjectName string `json:"project_name"`
	Builds      []struct {
		ID     string   `json:"id"`
		Main   string   `json:"main"`
		Binary string   `json:"binary"`
		Goos   []string `json:"goos"`
		Goarch []string `json:"goarch"`
	} `json:"builds"`
	Archives []struct {
		IDs []string `json:"ids"`
	} `json:"archives"`
}

func readYAML(t *testing.T, rel string, strict bool, out any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", rel))
	if err != nil {
		t.Fatal(err)
	}
	unmarshal := yaml.Unmarshal
	if strict {
		unmarshal = func(b []byte, v any, _ ...yaml.JSONOpt) error { return yaml.UnmarshalStrict(b, v) }
	}
	if err := unmarshal(data, out); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
}

func TestKrewManifestShape(t *testing.T) {
	var m krewManifest
	readYAML(t, "plugins/drain-check.yaml", true, &m)

	if m.APIVersion != "krew.googlecontainertools.github.com/v1alpha2" || m.Kind != "Plugin" {
		t.Fatalf("apiVersion/kind = %q/%q", m.APIVersion, m.Kind)
	}
	if m.Metadata.Name != "drain-check" {
		t.Errorf("name = %q", m.Metadata.Name)
	}
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(m.Spec.Version) {
		t.Errorf("version = %q, want vX.Y.Z", m.Spec.Version)
	}
	if m.Spec.Homepage == "" || m.Spec.ShortDescription == "" || m.Spec.Description == "" {
		t.Error("homepage, shortDescription and description are required")
	}
	want := map[string]bool{}
	for _, os := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			want[os+"/"+arch] = true
		}
	}
	sha := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, p := range m.Spec.Platforms {
		goos, goarch := p.Selector.MatchLabels["os"], p.Selector.MatchLabels["arch"]
		key := goos + "/" + goarch
		if !want[key] {
			t.Errorf("unexpected or duplicate platform %q", key)
		}
		delete(want, key)

		ext, bin := "tar.gz", "kubectl-drain_check"
		if goos == "windows" {
			ext, bin = "zip", "kubectl-drain_check.exe"
		}
		wantURI := fmt.Sprintf("https://github.com/Veinar/draincheck/releases/download/%s/draincheck_%s_%s_%s.%s",
			m.Spec.Version, strings.TrimPrefix(m.Spec.Version, "v"), goos, goarch, ext)
		if p.URI != wantURI {
			t.Errorf("%s: uri = %q, want %q", key, p.URI, wantURI)
		}
		if p.Bin != bin {
			t.Errorf("%s: bin = %q, want %q", key, p.Bin, bin)
		}
		// Until the release exists the hash is a visible placeholder, never a made-up digest.
		if !sha.MatchString(p.Sha256) && !strings.HasPrefix(p.Sha256, "REPLACE_") {
			t.Errorf("%s: sha256 = %q, want 64 hex characters or a REPLACE_ placeholder", key, p.Sha256)
		}
		files := map[string]bool{}
		for _, f := range p.Files {
			files[f.From] = true
		}
		if !files[bin] || !files["LICENSE"] {
			t.Errorf("%s: files must extract %s and LICENSE, got %v", key, bin, p.Files)
		}
	}
	for key := range want {
		t.Errorf("missing platform %q", key)
	}
}

// The manifest names kubectl-drain_check inside every archive, so the release
// config has to build it into the same archives as draincheck.
func TestGoreleaserShipsKubectlPluginBinary(t *testing.T) {
	var g goreleaserConfig
	readYAML(t, ".goreleaser.yaml", false, &g)

	// Archive names come from project_name; the manifest URIs depend on them.
	if g.ProjectName != "draincheck" {
		t.Errorf("project_name = %q, want draincheck", g.ProjectName)
	}

	ids := map[string]string{}
	for _, b := range g.Builds {
		if b.Main != "./cmd/draincheck" {
			t.Errorf("build %s: main = %q", b.ID, b.Main)
		}
		ids[b.Binary] = b.ID
	}
	for _, bin := range []string{"draincheck", "kubectl-drain_check"} {
		if ids[bin] == "" {
			t.Fatalf("no build produces %q", bin)
		}
	}
	if len(g.Archives) != 1 {
		t.Fatalf("archives = %d, want 1", len(g.Archives))
	}
	got := strings.Join(g.Archives[0].IDs, ",")
	if !strings.Contains(got, ids["draincheck"]) || !strings.Contains(got, ids["kubectl-drain_check"]) {
		t.Errorf("archive ids = %q, want both %q and %q", got, ids["draincheck"], ids["kubectl-drain_check"])
	}
}
