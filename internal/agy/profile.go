package agy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var profileRegex = regexp.MustCompile(`^agy(0[1-9]|1[0-5])$`)

type ProfileManager struct {
	profilesRoot   string
	defaultProfile string
}

func NewProfileManager(profilesRoot string, defaultProfile string) *ProfileManager {
	if profilesRoot == "" {
		profilesRoot = "/opt/data/home/agy-profiles"
	}
	if defaultProfile == "" {
		defaultProfile = "agy01"
	}
	return &ProfileManager{
		profilesRoot:   profilesRoot,
		defaultProfile: defaultProfile,
	}
}

func (m *ProfileManager) ListProfiles() ([]string, error) {
	entries, err := os.ReadDir(m.profilesRoot)
	if err != nil {
		return nil, err
	}

	var profiles []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if profileRegex.MatchString(name) {
			profiles = append(profiles, name)
		}
	}

	sort.Strings(profiles)
	return profiles, nil
}

func (m *ProfileManager) ValidateProfile(name string) bool {
	if !profileRegex.MatchString(name) {
		return false
	}
	profilePath := filepath.Join(m.profilesRoot, name)
	info, err := os.Stat(profilePath)
	return err == nil && info.IsDir()
}

func (m *ProfileManager) EnsureProfile(name string) (string, error) {
	if !profileRegex.MatchString(name) {
		return "", fmt.Errorf("invalid profile format: %s (expected agy01 to agy15)", name)
	}

	profilePath := filepath.Join(m.profilesRoot, name)
	if err := os.MkdirAll(profilePath, 0755); err != nil {
		return "", err
	}
	geminiPath := filepath.Join(profilePath, ".gemini")
	if err := os.MkdirAll(geminiPath, 0755); err != nil {
		return "", err
	}

	return profilePath, nil
}

func (m *ProfileManager) GetProfileHome(name string) string {
	return filepath.Join(m.profilesRoot, name)
}

func (m *ProfileManager) BuildEnv(name string) []string {
	profileHome := m.GetProfileHome(name)
	geminiHome := filepath.Join(profileHome, ".gemini")

	currentPath := os.Getenv("PATH")
	enhancedPath := "/opt/data/tools/go/bin:/opt/data/home/.local/bin:" + currentPath

	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "GEMINI_HOME=") || strings.HasPrefix(e, "PATH=") {
			continue
		}
		env = append(env, e)
	}

	env = append(env,
		fmt.Sprintf("HOME=%s", profileHome),
		fmt.Sprintf("GEMINI_HOME=%s", geminiHome),
		fmt.Sprintf("PATH=%s", enhancedPath),
	)

	return env
}
