package pod

import (
	"fmt"
	"os"
	"path/filepath"
)

// ManifestFile is the manifest's fixed name at the bundle root (pod-spec §7).
const ManifestFile = "pod.yaml"

// stateDir is the State-plane root within a bundle (pod-spec §7).
const stateDir = "state"

// Bundle is a loaded Pod Bundle: the manifest plus enough about the on-disk
// layout to validate references and tell thick from thin (pod-spec §7, §9.3).
// A Bundle is inert — it holds no runtime (pod-spec §10).
type Bundle struct {
	Root     string    // filesystem root of the bundle directory
	Manifest *Manifest // parsed pod.yaml
	Thick    bool      // true if a state/ directory is embedded (pod-spec §9.3)
}

// Load reads and validates a Pod Bundle directory: parses the manifest,
// validates it (ValidateManifest), and confirms every Definition-plane ref
// resolves to a file within the bundle. It does not run the runtime
// compatibility handshake — call CheckCompatibility separately (pod-spec §9.2),
// as it depends on the runtime version in play. LOAD is read-only and has no
// side effects (pod-spec §4).
func Load(root string) (*Bundle, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("bundle root %q is not a directory", root)
	}

	data, err := os.ReadFile(filepath.Join(root, ManifestFile))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ManifestFile, err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}

	b := &Bundle{Root: root, Manifest: m, Thick: dirExists(filepath.Join(root, stateDir))}

	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b, nil
}

// Validate runs manifest validation plus on-disk ref-existence checks, returning
// all problems found in one pass.
func (b *Bundle) Validate() error {
	var c collector

	if err := ValidateManifest(b.Manifest); err != nil {
		// ValidateManifest already returns ValidationErrors; fold them in.
		if ve, ok := err.(ValidationErrors); ok {
			c.errs = append(c.errs, ve...)
		} else {
			c.add("", err.Error())
		}
	}

	s := b.Manifest.Spec
	b.checkRefsExist(&c, "spec.agents", refPaths(s.Agents))
	b.checkRefsExist(&c, "spec.workflows", refPaths(s.Workflows))
	for i, d := range s.Deployments {
		if d.Ref != "" {
			b.checkRefExists(&c, fmt.Sprintf("spec.deployments[%d].ref", i), d.Ref)
		}
	}

	return c.result().ErrorOrNil()
}

// Materialization reports the bundle form (pod-spec §9.3).
func (b *Bundle) Materialization() string {
	if b.Thick {
		return "thick" // State embedded under state/
	}
	return "thin" // State referenced in live namespaces
}

func (b *Bundle) checkRefsExist(c *collector, field string, paths []string) {
	for i, p := range paths {
		b.checkRefExists(c, fmt.Sprintf("%s[%d].ref", field, i), p)
	}
}

func (b *Bundle) checkRefExists(c *collector, field, rel string) {
	if rel == "" {
		return // emptiness already reported by ValidateManifest
	}
	if err := checkRelPath(rel); err != nil {
		return // bad-path already reported by ValidateManifest
	}
	if !fileExists(filepath.Join(b.Root, filepath.FromSlash(rel))) {
		c.add(field, fmt.Sprintf("referenced file %q not found in bundle", rel))
	}
}

func refPaths(refs []Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Ref
	}
	return out
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
